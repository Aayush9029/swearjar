package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Aayush9029/swearjar/internal/agent"
	"github.com/Aayush9029/swearjar/internal/detector"
	_ "github.com/duckdb/duckdb-go/v2"
)

type Store struct {
	db          *sql.DB
	tx          *sql.Tx
	insertMsg   *sql.Stmt
	insertMatch *sql.Stmt
	nextID      int64
	startedAt   time.Time
}

func New(ctx context.Context) (*Store, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db, startedAt: time.Now()}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) init(ctx context.Context) error {
	for _, query := range []string{
		`CREATE TABLE messages (
			id BIGINT,
			agent VARCHAR,
			session VARCHAR,
			project VARCHAR,
			ts VARCHAR,
			chars BIGINT
		)`,
		`CREATE TABLE matches (
			message_id BIGINT,
			agent VARCHAR,
			session VARCHAR,
			project VARCHAR,
			ts VARCHAR,
			word VARCHAR,
			group_name VARCHAR,
			source VARCHAR
		)`,
	} {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	insertMsg, err := tx.PrepareContext(ctx, `INSERT INTO messages VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	insertMatch, err := tx.PrepareContext(ctx, `INSERT INTO matches VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = insertMsg.Close()
		_ = tx.Rollback()
		return err
	}
	s.tx = tx
	s.insertMsg = insertMsg
	s.insertMatch = insertMatch
	return nil
}

func (s *Store) Insert(ctx context.Context, msg agent.Message, result detector.Result) error {
	s.nextID++
	id := s.nextID
	if _, err := s.insertMsg.ExecContext(ctx, id, msg.Agent, msg.Session, msg.Project, msg.Timestamp, len(msg.Text)); err != nil {
		return err
	}
	for _, match := range result.Matches {
		if _, err := s.insertMatch.ExecContext(ctx, id, msg.Agent, msg.Session, msg.Project, msg.Timestamp, match.Word, match.Group, match.Source); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Report(ctx context.Context, scope string) (Report, error) {
	if err := s.flush(); err != nil {
		return Report{}, err
	}

	var totals Totals
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM messages),
			(SELECT count(*) FROM matches),
			(SELECT count(DISTINCT agent || ':' || session) FROM messages WHERE session <> ''),
			(SELECT coalesce(sum(chars), 0) FROM messages)
	`).Scan(&totals.Messages, &totals.Swears, &totals.Sessions, &totals.Chars); err != nil {
		return Report{}, err
	}
	totals.Rate = percent(totals.Swears, totals.Messages)

	agents, err := queryAgents(ctx, s.db)
	if err != nil {
		return Report{}, err
	}
	words, err := queryWords(ctx, s.db, totals.Swears)
	if err != nil {
		return Report{}, err
	}
	variants, err := queryVariants(ctx, s.db)
	if err != nil {
		return Report{}, err
	}
	sessions, err := querySessions(ctx, s.db)
	if err != nil {
		return Report{}, err
	}

	return Report{
		GeneratedAt: time.Now(),
		Duration:    time.Since(s.startedAt).Round(time.Millisecond).String(),
		Scope:       scope,
		Totals:      totals,
		Agents:      agents,
		Words:       words,
		Variants:    variants,
		Sessions:    sessions,
	}, nil
}

func (s *Store) Close() error {
	err := s.flush()
	if s.db != nil {
		if closeErr := s.db.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func (s *Store) flush() error {
	if s.tx == nil {
		return nil
	}
	if s.insertMsg != nil {
		_ = s.insertMsg.Close()
	}
	if s.insertMatch != nil {
		_ = s.insertMatch.Close()
	}
	err := s.tx.Commit()
	s.tx = nil
	s.insertMsg = nil
	s.insertMatch = nil
	return err
}

func queryAgents(ctx context.Context, db *sql.DB) ([]AgentRow, error) {
	rows, err := db.QueryContext(ctx, `
		WITH message_counts AS (
			SELECT agent, count(*) AS messages, count(DISTINCT session) AS sessions
			FROM messages
			GROUP BY agent
		),
		match_counts AS (
			SELECT agent, count(*) AS swears
			FROM matches
			GROUP BY agent
		)
		SELECT
			m.agent,
			m.messages,
			coalesce(c.swears, 0),
			m.sessions
		FROM message_counts m
		LEFT JOIN match_counts c USING (agent)
		ORDER BY coalesce(c.swears, 0) DESC, m.messages DESC, m.agent ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentRow
	for rows.Next() {
		var row AgentRow
		if err := rows.Scan(&row.Agent, &row.Messages, &row.Swears, &row.Sessions); err != nil {
			return nil, err
		}
		row.Rate = percent(row.Swears, row.Messages)
		out = append(out, row)
	}
	return out, rows.Err()
}

func queryWords(ctx context.Context, db *sql.DB, total int64) ([]WordRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT group_name, any_value(source), count(*) AS swears
		FROM matches
		GROUP BY group_name
		ORDER BY swears DESC, group_name ASC
		LIMIT 30
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WordRow
	for rows.Next() {
		var row WordRow
		if err := rows.Scan(&row.Group, &row.Source, &row.Count); err != nil {
			return nil, err
		}
		row.Share = percent(row.Count, total)
		out = append(out, row)
	}
	return out, rows.Err()
}

func queryVariants(ctx context.Context, db *sql.DB) ([]VariantRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT group_name, word, count(*) AS swears
		FROM matches
		GROUP BY group_name, word
		ORDER BY swears DESC, group_name ASC, word ASC
		LIMIT 80
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VariantRow
	for rows.Next() {
		var row VariantRow
		if err := rows.Scan(&row.Group, &row.Word, &row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func querySessions(ctx context.Context, db *sql.DB) ([]SessionRow, error) {
	rows, err := db.QueryContext(ctx, `
		WITH message_counts AS (
			SELECT agent, session, any_value(project) AS project, count(*) AS messages
			FROM messages
			WHERE session <> ''
			GROUP BY agent, session
		),
		match_counts AS (
			SELECT agent, session, count(*) AS swears
			FROM matches
			WHERE session <> ''
			GROUP BY agent, session
		)
		SELECT m.agent, m.session, coalesce(m.project, ''), m.messages, coalesce(c.swears, 0)
		FROM message_counts m
		LEFT JOIN match_counts c USING (agent, session)
		ORDER BY coalesce(c.swears, 0) DESC, m.messages DESC, m.agent ASC
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionRow
	for rows.Next() {
		var row SessionRow
		if err := rows.Scan(&row.Agent, &row.Session, &row.Project, &row.Messages, &row.Swears); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func percent(num, denom int64) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) / float64(denom) * 100
}

func Scan(ctx context.Context, adapters []agent.Adapter, opts agent.Options, scope string) (Report, error) {
	store, err := New(ctx)
	if err != nil {
		return Report{}, err
	}
	defer store.Close()

	d := detector.New()
	for _, adapter := range adapters {
		err := adapter.VisitMessages(ctx, opts, func(msg agent.Message) error {
			result := d.Detect(msg.Text)
			return store.Insert(ctx, msg, result)
		})
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", adapter.Name(), err)
		}
	}
	return store.Report(ctx, scope)
}
