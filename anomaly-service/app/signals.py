"""Named, de-standardized view of one scored request's model inputs.

`edge_features` holds z-scores for the numeric columns listed in
preprocessing.json and raw values (0/1 flags, fractions, one-hot) for the rest.
Reasons and scenarios need real quantities ("3 requests in 5 minutes", "8x the
median"), so numeric columns are mapped back with the stored mean/scale and the
`log1p` encoding is undone. Anything that cannot be recovered is NaN, and every
comparison with NaN is False, so a rule never fires on missing information.
"""

import math

NAN = float("nan")

# Numeric columns encode "no data" as 0 and say so in a companion flag. Without the
# flag they look like extreme z-scores (e.g. a first-ever transfer is "0 seconds
# after the previous one"), so they are ignored when the flag is off.
GATES = {
    "log_seconds_since_previous": "has_previous",
    "log_src_type_completed_median_amount": "has_type_completed_history",
    "log_src_type_completed_count": "has_type_completed_history",
    "log_amount_to_type_median": "type_amount_reference_reliable",
    "log_src_type_completed_mean_amount": "has_type_mean_history",
    "log_src_type_mean_count": "has_type_mean_history",
    "log_amount_to_type_mean": "type_mean_reference_reliable",
    "channel_prior_fraction": "has_previous",
    "hour_prior_fraction": "has_previous",
}


class Signals:
    def __init__(self, names, row, numeric_indices=(), mean=None, scale=None):
        self.names = list(names)
        self._index = {name: i for i, name in enumerate(self.names)}
        self._row = [float(v) for v in row]
        numeric = list(numeric_indices)
        self._numeric = {self.names[i]: k for k, i in enumerate(numeric)}
        self._mean = list(mean) if mean is not None and len(mean) == len(numeric) else None
        self._scale = list(scale) if scale is not None and len(scale) == len(numeric) else None

    def known(self, name):
        return name in self._index

    def is_numeric(self, name):
        return name in self._numeric

    def active(self, name):
        """False when the feature is only a placeholder because its history flag is off."""
        gate = GATES.get(name)
        return gate is None or not self.known(gate) or self.flag(gate)

    def flag(self, name):
        return self.known(name) and self._row[self._index[name]] >= 0.5

    def z(self, name):
        """Standardized value for numeric columns, NaN for others or when gated off."""
        if name not in self._numeric or not self.active(name):
            return NAN
        return self._row[self._index[name]]

    def raw(self, name):
        """Value in model units (log1p space for log_* columns), NaN if unknown or gated."""
        if not self.known(name) or not self.active(name):
            return NAN
        value = self._row[self._index[name]]
        if name not in self._numeric:
            return value
        if self._mean is None or self._scale is None:
            return NAN
        k = self._numeric[name]
        return value * self._scale[k] + self._mean[k]

    def qty(self, name):
        """Undo log1p: a count, an amount, or seconds. NaN when unknown."""
        raw = self.raw(name)
        if math.isnan(raw):
            return NAN
        return max(math.expm1(raw), 0.0)
