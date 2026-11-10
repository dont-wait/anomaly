package account

import (
	"net/url"
	"testing"
	"time"

	accountdomain "github.com/dont-wait/anomaly/internal/domain/account"
)

func reviewAccount(id, username, fullName, status string, at time.Time) *accountdomain.UserAccount {
	return &accountdomain.UserAccount{
		Id: id, Username: username,
		Customer: &accountdomain.Customer{
			Profile: accountdomain.CustomerProfile{FullName: fullName},
			ProfileChange: &accountdomain.ProfileChangeRequest{
				Id: "request-" + id, FullName: "Đổi tên " + username,
				Status: status, RequestedAt: at,
			},
		},
	}
}

func TestParseProfileChangeListParams(t *testing.T) {
	got, err := parseProfileChangeListParams(url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Page != 1 || got.PageSize != 10 || got.Status != "PENDING" || got.Search != "" {
		t.Fatalf("unexpected defaults: %#v", got)
	}
	got, err = parseProfileChangeListParams(url.Values{
		"page": {"2"}, "pageSize": {"5"}, "status": {"rejected"}, "search": {"  An  "},
	})
	if err != nil || got.Page != 2 || got.PageSize != 5 || got.Status != "REJECTED" || got.Search != "an" {
		t.Fatalf("unexpected parsing: %#v %v", got, err)
	}
	for _, values := range []url.Values{
		{"page": {"0"}}, {"page": {"x"}}, {"page": {"99999999999999999999999999999"}},
		{"pageSize": {"51"}}, {"pageSize": {"-1"}},
		{"status": {"UNKNOWN"}}, {"search": {string(make([]byte, 121))}},
	} {
		if _, err := parseProfileChangeListParams(values); err == nil {
			t.Fatalf("accepted invalid parameters: %#v", values)
		}
	}
}

func TestProfileChangePageStatusSearchAndOrder(t *testing.T) {
	now := time.Date(2026, 11, 7, 1, 0, 0, 0, time.UTC)
	accounts := []*accountdomain.UserAccount{
		reviewAccount("z", "nguyen", "Nguyễn A", "PENDING", now.Add(-time.Hour)),
		reviewAccount("b", "binh", "Bình B", "APPROVED", now.Add(time.Hour)),
		reviewAccount("a", "an", "An A", "PENDING", now),
		reviewAccount("c", "chi", "Chi C", "REJECTED", now),
		reviewAccount("x", "xa", "X", "PENDING", now),
		{Id: "missing-profile"},
	}
	params := profileChangeListParams{Page: 1, PageSize: 2, Status: "PENDING"}
	page := profileChangePage(accounts, params)
	if page.Total != 3 || page.TotalPages != 2 || len(page.Items) != 2 ||
		page.Items[0].AccountID != "a" || page.Items[1].AccountID != "x" {
		t.Fatalf("unexpected sorted page: %#v", page)
	}
	params.Page = 2
	page = profileChangePage(accounts, params)
	if len(page.Items) != 1 || page.Items[0].AccountID != "z" {
		t.Fatalf("unexpected second page: %#v", page)
	}
	params.Page = 100
	if page = profileChangePage(accounts, params); page.Total != 3 || len(page.Items) != 0 {
		t.Fatalf("out of range page must be empty: %#v", page)
	}
	params = profileChangeListParams{Page: 1, PageSize: 10, Status: "ALL", Search: "BÌNH"}
	page = profileChangePage(accounts, params)
	if page.Total != 1 || page.Items[0].AccountID != "b" {
		t.Fatalf("search failed: %#v", page)
	}
	params.Search = ""
	page = profileChangePage(accounts, params)
	if page.Total != 5 {
		t.Fatalf("all statuses should be included: %#v", page)
	}
}
