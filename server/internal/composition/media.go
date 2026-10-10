package composition

import (
	"github.com/rs/zerolog"

	"github.com/dont-wait/anomaly/internal/application/account/queries"
	"github.com/dont-wait/anomaly/internal/infrastructure/rustfs"
	handlermedia "github.com/dont-wait/anomaly/internal/presentation/http/handler/media"
)

func NewMediaHandler(repo *rustfs.MediaRepository, accounts queries.AccountQueryRepository, logger zerolog.Logger) *handlermedia.Handler {
	return handlermedia.NewHandler(logger, repo, accounts)
}
