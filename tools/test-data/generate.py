#!/usr/bin/env python3
"""Generate the reproducible A-F anomaly test dataset.

This script only creates a local JSON fixture. It does not call the API, write
to EventStoreDB, or create accounts in a running environment. A later seeder
can consume the fixture without changing the generated scenario data.
"""

from __future__ import annotations

import argparse
import json
import sys
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

from mimesis import Generic
from mimesis.enums import Locale


# History ends this long before generation, so a probe sent soon after import sees
# recent history (7-day mean, 24h counts, time since previous) but empty 2m/5m/1h
# velocity windows. Pass --reference-time to reproduce an older fixture exactly.
DEFAULT_PROBE_GAP = timedelta(hours=1)
DEFAULT_SEED = 20261110
DEFAULT_HISTORY_COUNT = 30
DEFAULT_HISTORY_DAYS = 7
TEST_PASSWORD = "Password123@"
NO_HISTORY_ALIASES = {"E"}
SOURCE_ALIASES = ("A", "B", "C", "D", "E", "F")
NAMESPACE = uuid.UUID("2d0e0e4d-b3d0-4a5a-8c2b-8dcf9ccf6f8b")
VIETNAM_TIMEZONE = timezone(timedelta(hours=7))


def history_times(seed: int, alias: str, reference_time: datetime,
                  history_count: int, history_days: int) -> list[datetime]:
    """Sample daytime slots with >=30-minute gaps, returning UTC timestamps."""
    if history_count < 0 or history_days <= 0:
        raise ValueError("history-count must be non-negative and history-days must be positive")
    if history_count == 0:
        return []
    end = reference_time.astimezone(VIETNAM_TIMEZONE)
    start = end - timedelta(days=history_days)
    rng = Generic(Locale.EN, seed=f"{seed}:schedule:{alias}").random
    candidates = []
    day = start.replace(hour=0, minute=0, second=0, microsecond=0)
    while day <= end:
        for hour in range(7, 22):
            slot = day.replace(hour=hour)
            # Adjacent hourly slots remain at least 30 minutes apart even
            # after jitter. Require the complete slot inside the history window.
            if slot >= start and slot + timedelta(minutes=30, seconds=10) < end:
                candidates.append(slot + timedelta(seconds=rng.randint(0, 1799)))
        day += timedelta(days=1)
    if history_count > len(candidates):
        raise ValueError(
            f"history-count {history_count} exceeds {len(candidates)} available daytime "
            "slots; increase --history-days or reduce --history-count"
        )
    return sorted(at.astimezone(timezone.utc) for at in rng.sample(candidates, history_count))


def default_reference_time(now: datetime | None = None) -> datetime:
    """End of the history window: DEFAULT_PROBE_GAP before now, whole seconds, UTC."""
    now = (now or datetime.now(timezone.utc)).astimezone(timezone.utc)
    return (now - DEFAULT_PROBE_GAP).replace(microsecond=0)


def parse_reference_time(value: str) -> datetime:
    normalized = value.strip().replace("Z", "+00:00")
    parsed = datetime.fromisoformat(normalized)
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def stable_uuid(*parts: str) -> str:
    return str(uuid.uuid5(NAMESPACE, ":".join(parts)))


def account_number(generic: Generic) -> str:
    return generic.random.generate_string_by_mask("9#############")


def cccd_number(generic: Generic) -> str:
    return generic.random.generate_string_by_mask("0###########")


def amount_for(alias: str, generic: Generic) -> int:
    if alias == "A":
        return generic.random.choice([10_000, 20_000, 30_000, 40_000, 50_000])
    return generic.random.choice([15_000, 20_000, 20_000, 25_000])


def scenario_for(alias: str) -> dict[str, Any]:
    scenarios: dict[str, dict[str, Any]] = {
        "A": {
            "name": "normal",
            "description": "Normal transfer to a familiar receiver on the familiar device and channel.",
            "probe": {
                "amount": 20_000,
                "receiver": "receiver-A-known",
                "device": "known",
                "platform": "mobile",
            },
        },
        "B": {
            "name": "amount_spike",
            "description": "Transfer 3-8x the typical amount to a familiar receiver.",
            "probe": {
                "amount": 100_000,
                "receiver": "receiver-B-known",
                "device": "known",
                "platform": "mobile",
                "typical_amount": 20_000,
                "multiple": 5,
            },
        },
        "C": {
            "name": "new_receiver",
            "description": "Transfer to a receiver never used by this source account.",
            "probe": {
                "amount": 20_000,
                "receiver": "receiver-C-new",
                "device": "known",
                "platform": "mobile",
            },
        },
        "D": {
            "name": "new_device",
            "description": "Transfer from a new device while receiver and channel stay familiar.",
            "probe": {
                "amount": 20_000,
                "receiver": "receiver-D-known",
                "device": "new",
                "platform": "mobile",
            },
        },
        "E": {
            "name": "velocity_burst",
            "description": "Six manual requests, 15 seconds apart; expected positive label starts at request 3.",
            "probe": {
                "amount": 20_000,
                "receiver": "receiver-E-known",
                "device": "known",
                "platform": "mobile",
                "requests": 6,
                "interval_seconds": 15,
                "expected_positive_from_request": 3,
            },
        },
        "F": {
            "name": "contextual_amount",
            "description": "Large amount on a less-used channel to a new receiver, with the familiar device.",
            "probe": {
                "amount": 100_000,
                "receiver": "receiver-F-new",
                "device": "known",
                "platform": "web",
                "typical_amount": 20_000,
                "multiple": 5,
            },
        },
    }
    return scenarios[alias]


def build_dataset(seed: int, reference_time: datetime, history_count: int, history_days: int) -> dict[str, Any]:
    generic = Generic(Locale.EN, seed=seed)

    accounts: list[dict[str, Any]] = []
    by_alias: dict[str, dict[str, Any]] = {}
    for alias in SOURCE_ALIASES:
        person_name = generic.person.full_name()
        account = {
            "id": f"source-{alias}",
            "alias": alias,
            "kind": "source",
            "case": scenario_for(alias)["name"],
            "username": f"anomaly.test.{alias.lower()}",
            "full_name": person_name,
            "cccd": cccd_number(generic),
            "email": f"anomaly.test.{alias.lower()}@example.test",
            "password": TEST_PASSWORD,
            "account_no": account_number(generic),
            "status": "active",
            "kyc_verified": True,
            "known_device_id": stable_uuid("device", alias, "known"),
            "new_device_id": stable_uuid("device", alias, "new"),
            "known_platform": "mobile",
            "starting_balance": 25_000_000,
            "history": [],
            "scenario": scenario_for(alias),
        }
        accounts.append(account)
        by_alias[account["id"]] = account

    receiver_specs = [
        ("receiver-A-known", "A"),
        ("receiver-B-known", "B"),
        ("receiver-C-known", "C"),
        ("receiver-C-new", "C"),
        ("receiver-D-known", "D"),
        ("receiver-E-known", "E"),
        ("receiver-F-known", "F"),
        ("receiver-F-new", "F"),
    ]
    for receiver_id, source_alias in receiver_specs:
        account = {
            "id": receiver_id,
            "alias": receiver_id,
            "kind": "receiver",
            "case": None,
            "username": None,
            "full_name": generic.person.full_name(),
            "cccd": cccd_number(generic),
            "email": f"{receiver_id}@example.test",
            "password": None,
            "account_no": account_number(generic),
            "status": "active",
            "kyc_verified": True,
            "known_device_id": None,
            "new_device_id": None,
            "known_platform": None,
            "starting_balance": 0,
            "history": [],
            "receiver_for": source_alias,
        }
        accounts.append(account)
        by_alias[receiver_id] = account

    for source_alias in SOURCE_ALIASES:
        source = by_alias[f"source-{source_alias}"]
        known_receiver = by_alias[f"receiver-{source_alias}-known"]
        source_history_count = 0 if source_alias in NO_HISTORY_ALIASES else history_count
        schedule = history_times(seed, source_alias, reference_time, source_history_count, history_days)
        for index, occurred_at in enumerate(schedule):
            completed_at = occurred_at + timedelta(seconds=10)
            amount = amount_for(source_alias, generic)
            transfer_id = stable_uuid("history", source["id"], str(index))
            transfer = {
                "id": transfer_id,
                "idempotency_key": stable_uuid("idempotency", transfer_id),
                "source_account": source["id"],
                "destination_account": known_receiver["id"],
                "amount": amount,
                "currency": "VND",
                "bank_code": "ANOMALY",
                "note": f"baseline {source_alias} {index + 1:02d}",
                "device_id": source["known_device_id"],
                "platform": source["known_platform"],
                "status": "success",
                "occurred_at": occurred_at.isoformat().replace("+00:00", "Z"),
                "completed_at": completed_at.isoformat().replace("+00:00", "Z"),
            }
            source["history"].append(transfer)
            known_receiver["history"].append(transfer_id)

    for account in accounts:
        if account["kind"] == "source":
            outgoing = sum(item["amount"] for item in account["history"])
            account["expected_balance_after_history"] = account["starting_balance"] - outgoing
            if account["expected_balance_after_history"] < 0:
                raise ValueError(
                    f"history for {account['alias']} exceeds starting balance; "
                    "increase the fixture balance or reduce --history-count"
                )
        else:
            # Receiver history stores IDs only; calculate incoming amounts from
            # the source records so the fixture remains easy to inspect.
            incoming = sum(
                transfer["amount"]
                for source in accounts
                if source["kind"] == "source"
                for transfer in source["history"]
                if transfer["destination_account"] == account["id"]
            )
            account["expected_balance_after_history"] = account["starting_balance"] + incoming

    return {
        "schema_version": 1,
        "dataset_id": "anomaly-manual-a-f-v2",
        "seed": seed,
        "reference_time": reference_time.isoformat().replace("+00:00", "Z"),
        "history_days": history_days,
        "history_per_source_account": history_count,
        "history_counts": {
            account["alias"]: len(account["history"])
            for account in accounts
            if account["kind"] == "source"
        },
        "probe_transactions_generated": False,
        "notes": [
            "This fixture contains baseline history only; manual probe transactions are intentionally absent.",
            "Account E intentionally has no baseline history; use VelocityBurstUser for its six-request probe.",
            "The receiver/network expansion is intentionally out of scope for this dataset version.",
            "The fixture is not a direct database dump. A development/test seeder must translate it into canonical events.",
        ],
        "accounts": accounts,
    }


def write_dataset(dataset: dict[str, Any], output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(dataset, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("tools/test-data/output/anomaly-a-f.json"))
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED)
    parser.add_argument(
        "--reference-time",
        help="end of the history window (RFC3339); default: one hour before now",
    )
    parser.add_argument("--history-count", type=int, default=DEFAULT_HISTORY_COUNT)
    parser.add_argument("--history-days", type=int, default=DEFAULT_HISTORY_DAYS)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.history_count <= 0 or args.history_days <= 0:
        print("history-count and history-days must be positive", file=sys.stderr)
        return 2
    try:
        reference_time = (
            parse_reference_time(args.reference_time)
            if args.reference_time
            else default_reference_time()
        )
    except ValueError as error:
        print(f"invalid --reference-time: {error}", file=sys.stderr)
        return 2

    try:
        dataset = build_dataset(args.seed, reference_time, args.history_count, args.history_days)
    except ValueError as error:
        print(str(error), file=sys.stderr)
        return 2
    write_dataset(dataset, args.output)
    source_accounts = [account for account in dataset["accounts"] if account["kind"] == "source"]
    receiver_accounts = [account for account in dataset["accounts"] if account["kind"] == "receiver"]
    print(f"Generated {len(source_accounts)} source accounts and {len(receiver_accounts)} receiver accounts")
    print(f"Generated {sum(len(account['history']) for account in source_accounts)} baseline transactions")
    print(f"Wrote {args.output} (reference time {dataset['reference_time']})")
    for account in source_accounts:
        print(f"  {account['alias']}: case={account['case']}, history={len(account['history'])}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
