package seeder

import (
	"context"
	"fmt"
)

func init() {
	seeders.register("020_loan_catalog", seedLoanCatalog)
}

func seedLoanCatalog(ctx context.Context, deps Dependencies) error {
	if deps.LoanCatalog == nil {
		return nil
	}
	_, err := SeedLoanCatalog(ctx, deps.LoanCatalog)
	return err
}

// SeedLoanCatalog is shared by the CLI and the admin development endpoint.
func SeedLoanCatalog(ctx context.Context, catalog LoanCatalogSeeder) (bool, error) {
	if catalog == nil {
		return false, fmt.Errorf("loan catalog seeder is not configured")
	}
	created, err := catalog.SeedDefaultPackage(ctx)
	if err != nil {
		return false, fmt.Errorf("seed default loan package: %w", err)
	}
	return created, nil
}
