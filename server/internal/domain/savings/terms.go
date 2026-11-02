package savings

import (
	"errors"
	"strings"
)

// TermsSnapshot is the content of the currently published product terms.
// Product content is maintained in MongoDB, not in Go source code.
type TermsSnapshot struct {
	Version      string         `json:"version" bson:"version"`
	Title        string         `json:"title" bson:"title"`
	Introduction string         `json:"introduction" bson:"introduction"`
	Sections     []TermsSection `json:"sections" bson:"sections"`
}

type TermsSection struct {
	Title string `json:"title" bson:"title"`
	Body  string `json:"body" bson:"body"`
}

var ErrTermsUnavailable = errors.New("savings product terms unavailable")

func (t TermsSnapshot) Validate() error {
	if strings.TrimSpace(t.Version) == "" || strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Introduction) == "" || len(t.Sections) == 0 || len(t.Sections) > 30 {
		return ErrTermsUnavailable
	}
	for _, section := range t.Sections {
		if strings.TrimSpace(section.Title) == "" || strings.TrimSpace(section.Body) == "" {
			return ErrTermsUnavailable
		}
	}
	return nil
}
