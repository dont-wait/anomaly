package timeutil

import "time"

// VietnamLocation is the business timezone used for date-only calculations.
// Vietnam has no daylight saving time, so a fixed +07:00 offset avoids a
// dependency on the host's tzdata while remaining correct for current dates.
var VietnamLocation = time.FixedZone("ICT", 7*60*60)
