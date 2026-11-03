package catalog

import (
	"errors"
	"testing"
)

func TestPackageValidate(t *testing.T) {
	loan := Package{Kind: KindLoan, Name: "Vay", MinAmount: 1, MaxAmount: 2, Terms: []TermOption{{Months: 12}}}
	save := Package{Kind: KindSavings, Name: "Gửi", MinAmount: 1_000_000, Terms: []TermOption{{Months: 12, AnnualRateBps: 525}}}
	if err := loan.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := save.Validate(); err != nil {
		t.Fatal(err)
	}
	mutate := func(base Package, change func(*Package)) Package {
		base.Terms = append([]TermOption(nil), base.Terms...)
		change(&base)
		return base
	}
	bad := map[string]Package{
		"no name":         mutate(loan, func(p *Package) { p.Name = " " }),
		"no terms":        mutate(loan, func(p *Package) { p.Terms = nil }),
		"repeated term":   mutate(loan, func(p *Package) { p.Terms = []TermOption{{Months: 12}, {Months: 12}} }),
		"loan max < min":  mutate(loan, func(p *Package) { p.MaxAmount = 0 }),
		"loan term":       mutate(loan, func(p *Package) { p.Terms[0].Months = 7 }),
		"loan rate":       mutate(loan, func(p *Package) { p.Terms[0].AnnualRateBps = 100 }),
		"below floor":     mutate(save, func(p *Package) { p.MinAmount = 999_999 }),
		"savings maximum": mutate(save, func(p *Package) { p.MaxAmount = 5_000_000 }),
		"savings term":    mutate(save, func(p *Package) { p.Terms[0].Months = 1 }),
		"zero rate":       mutate(save, func(p *Package) { p.Terms[0].AnnualRateBps = 0 }),
		"rate above 100":  mutate(save, func(p *Package) { p.Terms[0].AnnualRateBps = 10_001 }),
		"unknown kind":    mutate(loan, func(p *Package) { p.Kind = "x" }),
	}
	for name, p := range bad {
		if err := p.Validate(); !errors.Is(err, ErrInvalidPackage) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
