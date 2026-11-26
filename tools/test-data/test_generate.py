"""Offline tests: python -m unittest discover -s tools/test-data."""

import unittest
from datetime import datetime, timedelta, timezone

from generate import (
    VIETNAM_TIMEZONE,
    build_dataset,
    default_reference_time,
    history_times,
    parse_reference_time,
)


class HistoryScheduleTests(unittest.TestCase):
    def setUp(self):
        self.reference = parse_reference_time("2026-11-11T12:00:00Z")

    def test_reproducible_and_account_specific(self):
        first = history_times(42, "A", self.reference, 30, 7)
        self.assertEqual(first, history_times(42, "A", self.reference, 30, 7))
        self.assertNotEqual(first, history_times(42, "B", self.reference, 30, 7))
        self.assertNotEqual(first, history_times(43, "A", self.reference, 30, 7))

    def test_daytime_window_spacing_and_variety(self):
        for alias in "ABCDEF":
            times = history_times(20261110, alias, self.reference, 30, 7)
            self.assertEqual(len(times), 30)
            for at in times:
                self.assertGreaterEqual(at, self.reference - timedelta(days=7))
                self.assertLess(at + timedelta(seconds=10), self.reference)
                self.assertIn(at.astimezone(VIETNAM_TIMEZONE).hour, range(7, 22))
            gaps = [b - a for a, b in zip(times, times[1:])]
            self.assertTrue(all(gap >= timedelta(minutes=30) for gap in gaps))
            self.assertGreater(len(set(gaps)), 1)
            counts = {}
            for at in times:
                day = at.astimezone(VIETNAM_TIMEZONE).date()
                counts[day] = counts.get(day, 0) + 1
            self.assertGreater(len(set(counts.values())), 1)

    def test_reject_impossible_density(self):
        with self.assertRaises(ValueError):
            history_times(42, "A", self.reference, 1000, 1)

    def test_dataset_counts_and_credentials(self):
        dataset = build_dataset(42, self.reference, 30, 7)
        self.assertEqual(dataset, build_dataset(42, self.reference, 30, 7))
        self.assertEqual(len(dataset["accounts"]), 14)
        for account in dataset["accounts"]:
            if account["kind"] == "source":
                expected = 0 if account["alias"] == "E" else 30
                self.assertEqual(len(account["history"]), expected)
                self.assertEqual(account["password"], "Password123@")


    def test_default_reference_time_is_one_hour_before_now(self):
        now = datetime(2026, 11, 25, 10, 30, 15, 987654, tzinfo=timezone.utc)
        self.assertEqual(
            default_reference_time(now),
            datetime(2026, 11, 25, 9, 30, 15, tzinfo=timezone.utc),
        )
        # History therefore ends at least an hour before a probe sent "now".
        for at in history_times(42, "A", default_reference_time(now), 30, 7):
            self.assertLess(at, now - timedelta(hours=1))


if __name__ == "__main__":
    unittest.main()
