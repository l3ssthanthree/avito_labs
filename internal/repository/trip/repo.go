package trip

import sq "github.com/Masterminds/squirrel"

type Repository struct {
	database dbExecutor
	builder  sq.StatementBuilderType
}

func New(database dbExecutor) *Repository {
	return &Repository{
		database: database,
		builder:  sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
	}
}
