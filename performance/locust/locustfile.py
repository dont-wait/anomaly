"""Locust scenarios for the real-time alert and audit-trail flows.

The scenarios intentionally exercise public HTTP APIs. Kafka lag, model latency,
and database resource usage should be collected from the corresponding runtime
metrics while Locust measures the user-visible latency.

Required environment variables for the alert scenario:

    LOCUST_CUSTOMER_CCCD
    LOCUST_CUSTOMER_PASSWORD
    LOCUST_DESTINATION_ACCOUNT

Required environment variables for the audit scenario:

    LOCUST_ADMIN_CCCD
    LOCUST_ADMIN_PASSWORD
    LOCUST_AUDIT_ACCOUNT_ID

Example:

    locust -f performance/locust/locustfile.py --host http://localhost:8080
"""

from __future__ import annotations

import os
import json
import time
import uuid
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import gevent
import requests
from gevent.event import Event
from gevent.queue import Empty, Queue
from locust import HttpUser, between, task
from locust.exception import StopUser
from dotenv import load_dotenv


load_dotenv(Path(__file__).with_name(".env"))


def _env(name: str, default: str = "") -> str:
    return os.getenv(name, default).strip()


@dataclass(frozen=True)
class Settings:
    customer_cccd: str = _env("LOCUST_CUSTOMER_CCCD")
    customer_password: str = _env("LOCUST_CUSTOMER_PASSWORD")
    admin_cccd: str = _env("LOCUST_ADMIN_CCCD")
    admin_password: str = _env("LOCUST_ADMIN_PASSWORD")
    destination_account: str = _env("LOCUST_DESTINATION_ACCOUNT")
    audit_account_id: str = _env("LOCUST_AUDIT_ACCOUNT_ID")
    transfer_amount: int = int(_env("LOCUST_TRANSFER_AMOUNT", "1000"))
    transfer_bank_code: str = _env("LOCUST_TRANSFER_BANK_CODE", "ANOMALY")
    poll_interval: float = float(_env("LOCUST_ALERT_POLL_INTERVAL", "0.5"))
    alert_timeout: float = float(_env("LOCUST_ALERT_TIMEOUT", "30"))
    audit_from: str = _env("LOCUST_AUDIT_FROM", "2026-01-01")
    audit_to: str = _env("LOCUST_AUDIT_TO", "2030-01-01")
    audit_limit: int = int(_env("LOCUST_AUDIT_LIMIT", "20"))
    confirm_without_otp: bool = _env("LOCUST_CONFIRM_WITHOUT_OTP", "true").lower() == "true"


SETTINGS = Settings()


def _payload(response: Any) -> dict[str, Any]:
    """Return the application payload for both direct and wrapped responses."""
    try:
        value = response.json()
    except ValueError:
        return {}
    if not isinstance(value, dict):
        return {}
    data = value.get("data")
    return data if isinstance(data, dict) else value


class AuthenticatedUser(HttpUser):
    abstract = True
    host = _env("LOCUST_HOST", "http://localhost:8080")
    wait_time = between(1, 3)

    def _login(self, cccd: str, password: str, name: str) -> str:
        if not cccd or not password:
            raise StopUser(f"missing credentials for {name}")
        with self.client.post(
            "/api/auth/login",
            json={"cccdNumber": cccd, "password": password},
            name=name,
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"login returned HTTP {response.status_code}")
                raise StopUser(f"login failed for {name}")
            token = _payload(response).get("token")
            if not isinstance(token, str) or not token:
                response.failure("login response did not contain data.token")
                raise StopUser(f"login response missing token for {name}")
            return token

    @staticmethod
    def _headers(token: str) -> dict[str, str]:
        return {"Authorization": f"Bearer {token}"}

    def _record_custom_latency(
        self,
        name: str,
        started: float,
        *,
        response_length: int = 0,
        error: Exception | None = None,
    ) -> None:
        """Add an end-to-end measurement to Locust's request statistics."""
        elapsed = time.perf_counter() - started
        self.environment.events.request.fire(
            request_type="CUSTOM",
            name=name,
            response_time=elapsed * 1000,
            response_length=response_length,
            response=None,
            start_time=time.time() - elapsed,
            url=name,
            exception=error,
            context={},
        )


class AlertFlowMixin:
    def _start_alert_credentials(self) -> None:
        self.customer_token = self._login(
            SETTINGS.customer_cccd,
            SETTINGS.customer_password,
            "POST /api/auth/login [customer]",
        )
        self.admin_token = self._login(
            SETTINGS.admin_cccd,
            SETTINGS.admin_password,
            "POST /api/auth/login [admin]",
        )
        if not SETTINGS.destination_account:
            raise StopUser("LOCUST_DESTINATION_ACCOUNT is required")

    def _create_transfer(self) -> str | None:
        key = str(uuid.uuid4())
        body = {
            "toBankCode": SETTINGS.transfer_bank_code,
            "toAccountNo": SETTINGS.destination_account,
            "amount": SETTINGS.transfer_amount,
            "note": "locust realtime alert",
        }
        headers = {
            **self._headers(self.customer_token),
            "Idempotency-Key": key,
            "X-Platform": "locust",
        }
        with self.client.post(
            "/api/transfers",
            json=body,
            headers=headers,
            name="POST /api/transfers [realtime alert]",
            catch_response=True,
        ) as response:
            if response.status_code != 201:
                response.failure(f"create transfer returned HTTP {response.status_code}")
                return None
            payload = _payload(response)
            transaction_id = payload.get("transferId") or payload.get("id")
            if not isinstance(transaction_id, str) or not transaction_id:
                response.failure("create transfer response missing transferId")
                return None
            return transaction_id

    def _find_assessment(self, transaction_id: str) -> dict[str, Any] | None:
        with self.client.get(
            f"/api/admin/risk-assessments/transactions/{transaction_id}",
            headers=self._headers(self.admin_token),
            name="GET /api/admin/risk-assessments/transactions/{id}",
            catch_response=True,
        ) as response:
            if response.status_code == 404:
                response.success()
                return None
            if response.status_code != 200:
                response.failure(f"risk assessment returned HTTP {response.status_code}")
                return None
            payload = _payload(response)
            return payload if payload else None

    def _confirm_transfer(self, transaction_id: str) -> bool:
        with self.client.post(
            f"/api/transfers/{transaction_id}/confirm",
            json={"otp": ""},
            headers=self._headers(self.customer_token),
            name="POST /api/transfers/{id}/confirm [OTP disabled]",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"confirm transfer returned HTTP {response.status_code}")
                return False
            return True


class RealtimeAlertUser(AlertFlowMixin, AuthenticatedUser):
    """Create transfers and measure time until their risk assessment is visible."""

    weight = 2

    def on_start(self) -> None:
        self._start_alert_credentials()

    @task
    def create_transfer_and_wait_for_risk(self) -> None:
        transaction_id = self._create_transfer()
        if not transaction_id:
            return

        started = time.perf_counter()
        deadline = started + SETTINGS.alert_timeout
        while time.perf_counter() < deadline:
            assessment = self._find_assessment(transaction_id)
            if assessment is not None:
                self._record_custom_latency(
                    "E2E transaction -> risk decision",
                    started,
                    response_length=len(str(assessment)),
                )
                if SETTINGS.confirm_without_otp:
                    self._confirm_transfer(transaction_id)
                return
            time.sleep(SETTINGS.poll_interval)

        self._record_custom_latency(
            "E2E transaction -> risk decision",
            started,
            error=TimeoutError(
                f"risk decision not visible for transaction {transaction_id}"
            ),
        )

class RealtimeAlertSseUser(AlertFlowMixin, AuthenticatedUser):
    """Optional SSE scenario; select it explicitly with --user-classes."""

    weight = 0
    wait_time = between(1, 3)

    def on_start(self) -> None:
        self._start_alert_credentials()
        self._sse_stop = Event()
        self._sse_events: Queue[dict[str, Any]] = Queue()
        self._sse_greenlet = gevent.spawn(self._read_sse)

    def on_stop(self) -> None:
        if hasattr(self, "_sse_stop"):
            self._sse_stop.set()
        if hasattr(self, "_sse_greenlet"):
            self._sse_greenlet.kill(block=False)

    @task
    def create_transfer_and_wait_for_sse(self) -> None:
        transaction_id = self._create_transfer()
        if not transaction_id:
            return
        started = time.perf_counter()
        deadline = started + SETTINGS.alert_timeout
        while time.perf_counter() < deadline:
            try:
                item = self._sse_events.get(timeout=SETTINGS.poll_interval)
            except Empty:
                continue
            if item.get("transactionId") != transaction_id:
                continue
            self._record_custom_latency(
                "E2E transaction -> SSE risk alert",
                started,
                response_length=len(str(item)),
            )
            if SETTINGS.confirm_without_otp:
                self._confirm_transfer(transaction_id)
            return
        self._record_custom_latency(
            "E2E transaction -> SSE risk alert",
            started,
            error=TimeoutError(f"SSE alert not received for transaction {transaction_id}"),
        )

    def _read_sse(self) -> None:
        url = f"{self.host}/api/admin/monitor/stream"
        while not self._sse_stop.is_set():
            try:
                with requests.get(
                    url,
                    headers=self._headers(self.admin_token),
                    stream=True,
                    timeout=(5, None),
                ) as response:
                    if response.status_code != 200:
                        self._sse_stop.wait(1)
                        continue
                    data_lines: list[str] = []
                    for raw_line in response.iter_lines(decode_unicode=True):
                        if self._sse_stop.is_set():
                            return
                        line = raw_line or ""
                        if line.startswith("data:"):
                            data_lines.append(line[5:].strip())
                        elif not line and data_lines:
                            try:
                                value = json.loads("".join(data_lines))
                            except ValueError:
                                value = None
                            if isinstance(value, dict):
                                self._sse_events.put(value)
                            data_lines.clear()
            except requests.RequestException:
                self._sse_stop.wait(1)


class AuditUser(AuthenticatedUser):
    """Exercise admin monitor, account timeline, event explorer, and history APIs."""

    weight = 1

    def on_start(self) -> None:
        self.admin_token = self._login(
            SETTINGS.admin_cccd,
            SETTINGS.admin_password,
            "POST /api/auth/login [admin]",
        )
        self.customer_token = self._login(
            SETTINGS.customer_cccd,
            SETTINGS.customer_password,
            "POST /api/auth/login [customer history]",
        )
        if not SETTINGS.audit_account_id:
            raise StopUser("LOCUST_AUDIT_ACCOUNT_ID is required")
        self.timeline_cursor: str | None = None
        self.event_cursor: str | None = None

    @task(3)
    def monitor_snapshot(self) -> None:
        self.client.get(
            "/api/admin/monitor",
            params={"range": "24h", "handled": "all"},
            headers=self._headers(self.admin_token),
            name="GET /api/admin/monitor",
        )

    @task(3)
    def account_timeline(self) -> None:
        params: dict[str, str | int] = {
            "from": SETTINGS.audit_from,
            "to": SETTINGS.audit_to,
            "category": "all",
            "limit": SETTINGS.audit_limit,
        }
        if self.timeline_cursor:
            params["cursor"] = self.timeline_cursor
        with self.client.get(
            f"/api/admin/audit/accounts/{SETTINGS.audit_account_id}/events",
            params=params,
            headers=self._headers(self.admin_token),
            name="GET /api/admin/audit/accounts/{id}/events",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"timeline returned HTTP {response.status_code}")
                return
            payload = _payload(response)
            next_cursor = payload.get("nextCursor")
            self.timeline_cursor = next_cursor if isinstance(next_cursor, str) and next_cursor else None

    @task(2)
    def event_explorer(self) -> None:
        params: dict[str, str | int] = {
            "account": SETTINGS.audit_account_id,
            "category": "transfer",
            "from": SETTINGS.audit_from,
            "to": SETTINGS.audit_to,
            "limit": SETTINGS.audit_limit,
        }
        if self.event_cursor:
            params["cursor"] = self.event_cursor
        with self.client.get(
            "/api/admin/audit/events",
            params=params,
            headers=self._headers(self.admin_token),
            name="GET /api/admin/audit/events",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"event explorer returned HTTP {response.status_code}")
                return
            payload = _payload(response)
            next_cursor = payload.get("nextCursor")
            self.event_cursor = next_cursor if isinstance(next_cursor, str) and next_cursor else None

    @task(2)
    def transaction_history(self) -> None:
        # Transaction history requires RFC3339 bounds within the last three
        # months. Audit endpoints intentionally keep their own date-only range.
        now = datetime.now(timezone.utc)
        with self.client.get(
            "/api/transactions",
            params={
                "direction": "all",
                "from": (now - timedelta(days=7)).isoformat(),
                "to": now.isoformat(),
                "limit": SETTINGS.audit_limit,
            },
            headers=self._headers(self.customer_token),
            name="GET /api/transactions",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                payload = _payload(response)
                response.failure(
                    f"transaction history HTTP {response.status_code}: "
                    f"{payload.get('code', 'UNKNOWN_ERROR')}"
                )

    @task(1)
    def event_explorer_detail(self) -> None:
        with self.client.get(
            "/api/admin/audit/events",
            params={"account": SETTINGS.audit_account_id, "category": "transfer", "limit": 1},
            headers=self._headers(self.admin_token),
            name="GET /api/admin/audit/events [detail lookup]",
            catch_response=True,
        ) as response:
            if response.status_code != 200:
                response.failure(f"event lookup returned HTTP {response.status_code}")
                return
            payload = _payload(response)
            items = payload.get("items", [])
            if not isinstance(items, list) or not items:
                return
            event_id = items[0].get("eventId") if isinstance(items[0], dict) else None
            if not event_id:
                return
        self.client.get(
            f"/api/admin/audit/events/{event_id}",
            headers=self._headers(self.admin_token),
            name="GET /api/admin/audit/events/{id}",
        )
