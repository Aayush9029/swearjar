package analytics

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aayush9029/swearjar/internal/agent"
	"github.com/Aayush9029/swearjar/internal/detector"
	"github.com/duckdb/duckdb-go/v2"
	"golang.org/x/sync/errgroup"
)

type Store struct {
	db               *sql.DB
	mu               sync.Mutex
	nextID           int64
	startedAt        time.Time
	indexed          bool
	runID            string
	since            *time.Time
	pendingMessages  []messageRow
	pendingMatches   []matchRow
	completedSources map[string]agent.Source
}

const (
	flushRows    = 4096
	indexVersion = "exact-v1"
	TopWords     = 5
)

type messageRow struct {
	ID        int64
	SourceKey string
	Agent     string
	Session   string
	Project   string
	Timestamp string
	Chars     int64
}

type matchRow struct {
	MessageID int64
	SourceKey string
	Agent     string
	Session   string
	Project   string
	Timestamp string
	Word      string
	Group     string
}

func New(ctx context.Context) (*Store, error) {
	return newStore(ctx, "", false, nil)
}

func NewIndexed(ctx context.Context, since *time.Time) (*Store, error) {
	path, err := indexPath()
	if err != nil {
		return nil, err
	}
	return newStore(ctx, path, true, since)
}

func newStore(ctx context.Context, path string, indexed bool, since *time.Time) (*Store, error) {
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	store := &Store{
		db:               db,
		startedAt:        time.Now(),
		indexed:          indexed,
		runID:            fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()),
		since:            since,
		completedSources: map[string]agent.Source{},
	}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) init(ctx context.Context) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS sources (
			source_key VARCHAR,
			agent VARCHAR,
			path VARCHAR,
			session VARCHAR,
			project VARCHAR,
			size BIGINT,
			mod_time BIGINT,
			index_version VARCHAR,
			indexed_at TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS run_sources (
			run_id VARCHAR,
			source_key VARCHAR
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id BIGINT,
			source_key VARCHAR,
			agent VARCHAR,
			session VARCHAR,
			project VARCHAR,
			ts VARCHAR,
			chars BIGINT
		)`,
		`CREATE TABLE IF NOT EXISTS matches (
			message_id BIGINT,
			source_key VARCHAR,
			agent VARCHAR,
			session VARCHAR,
			project VARCHAR,
			ts VARCHAR,
			word VARCHAR,
			group_name VARCHAR
		)`,
	} {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(max(id), 0) FROM messages`).Scan(&s.nextID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE sources ADD COLUMN IF NOT EXISTS index_version VARCHAR`); err != nil {
		return err
	}
	if s.indexed {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM run_sources`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Insert(ctx context.Context, msg agent.Message, result detector.Result) error {
	s.nextID++
	id := s.nextID
	sourceKey := messageSourceKey(msg, id)
	s.pendingMessages = append(s.pendingMessages, messageRow{
		ID:        id,
		SourceKey: sourceKey,
		Agent:     msg.Agent,
		Session:   msg.Session,
		Project:   msg.Project,
		Timestamp: msg.Timestamp,
		Chars:     int64(len(msg.Text)),
	})
	for _, match := range result.Matches {
		s.pendingMatches = append(s.pendingMatches, matchRow{
			MessageID: id,
			SourceKey: sourceKey,
			Agent:     msg.Agent,
			Session:   msg.Session,
			Project:   msg.Project,
			Timestamp: msg.Timestamp,
			Word:      match.Word,
			Group:     match.Group,
		})
	}
	if len(s.pendingMessages)+len(s.pendingMatches) >= flushRows {
		return s.Flush(ctx)
	}
	return nil
}

func (s *Store) BeginSource(ctx context.Context, src agent.Source) (bool, error) {
	if !s.indexed || src.Path == "" {
		return true, nil
	}
	key := sourceKey(src)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.addRunSourceLocked(ctx, key); err != nil {
		return false, err
	}

	var size int64
	var modTime int64
	var version string
	err := s.db.QueryRowContext(ctx, `SELECT size, mod_time, coalesce(index_version, '') FROM sources WHERE source_key = ? ORDER BY indexed_at DESC LIMIT 1`, key).Scan(&size, &modTime, &version)
	if err == nil && size == src.Size && modTime == src.ModTime && version == indexVersion {
		return false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	for _, query := range []string{
		`DELETE FROM matches WHERE source_key = ?`,
		`DELETE FROM messages WHERE source_key = ?`,
		`DELETE FROM sources WHERE source_key = ?`,
	} {
		if _, err := s.db.ExecContext(ctx, query, key); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Store) FinishSource(ctx context.Context, src agent.Source) error {
	if !s.indexed || src.Path == "" {
		return nil
	}
	key := sourceKey(src)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.addRunSourceLocked(ctx, key); err != nil {
		return err
	}
	s.completedSources[key] = src
	return nil
}

func (s *Store) addRunSourceLocked(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO run_sources
		SELECT ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM run_sources WHERE run_id = ? AND source_key = ?
		)
	`, s.runID, key, s.runID, key)
	return err
}

func (s *Store) Report(ctx context.Context, scope string) (Report, error) {
	if err := s.Flush(ctx); err != nil {
		return Report{}, err
	}
	if err := s.finalizeSources(ctx); err != nil {
		return Report{}, err
	}
	if err := s.createScopedViews(ctx); err != nil {
		return Report{}, err
	}

	var totals Totals
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM scoped_messages),
			(SELECT count(*) FROM scoped_matches),
			(SELECT count(DISTINCT agent || ':' || session) FROM scoped_messages WHERE session <> ''),
			(SELECT coalesce(sum(chars), 0) FROM scoped_messages)
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

func (s *Store) finalizeSources(ctx context.Context) error {
	if !s.indexed || len(s.completedSources) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, src := range s.completedSources {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO sources
			(source_key, agent, path, session, project, size, mod_time, index_version, indexed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, now())
		`, key, src.Agent, src.Path, src.Session, src.Project, src.Size, src.ModTime, indexVersion)
		if err != nil {
			return err
		}
		delete(s.completedSources, key)
	}
	return nil
}

func (s *Store) createScopedViews(ctx context.Context) error {
	for _, query := range []string{
		`DROP VIEW IF EXISTS scoped_matches`,
		`DROP VIEW IF EXISTS scoped_messages`,
	} {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}

	sourceSQL := `SELECT * FROM messages`
	if s.indexed {
		sourceSQL = `SELECT m.* FROM messages m JOIN run_sources rs ON rs.source_key = m.source_key WHERE rs.run_id = ` + sqlString(s.runID)
	}
	if s.since != nil {
		filter := `(ts = '' OR try_cast(ts AS TIMESTAMPTZ) IS NULL OR try_cast(ts AS TIMESTAMPTZ) >= try_cast(` + sqlString(s.since.Format(time.RFC3339Nano)) + ` AS TIMESTAMPTZ))`
		if strings.Contains(sourceSQL, " WHERE ") {
			sourceSQL += " AND " + filter
		} else {
			sourceSQL += " WHERE " + filter
		}
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TEMP VIEW scoped_messages AS `+sourceSQL); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		CREATE TEMP VIEW scoped_matches AS
		SELECT mt.*
		FROM matches mt
		JOIN scoped_messages sm ON sm.id = mt.message_id
	`)
	return err
}

func (s *Store) Close() error {
	err := s.Flush(context.Background())
	if s.db != nil {
		if closeErr := s.db.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func (s *Store) Flush(ctx context.Context) error {
	if len(s.pendingMessages) == 0 && len(s.pendingMatches) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.appendRows(ctx, s.pendingMessages, s.pendingMatches); err != nil {
		return err
	}
	s.pendingMessages = s.pendingMessages[:0]
	s.pendingMatches = s.pendingMatches[:0]
	return nil
}

func (s *Store) appendRows(ctx context.Context, messages []messageRow, matches []matchRow) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Raw(func(raw any) error {
		driverConn, ok := raw.(driver.Conn)
		if !ok {
			return fmt.Errorf("duckdb raw connection has unexpected type %T", raw)
		}
		if err := appendMessages(driverConn, messages); err != nil {
			return err
		}
		return appendMatches(driverConn, matches)
	})
}

func appendMessages(conn driver.Conn, rows []messageRow) error {
	if len(rows) == 0 {
		return nil
	}
	appender, err := duckdb.NewAppenderFromConn(conn, "", "messages")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := appender.AppendRow(row.ID, row.SourceKey, row.Agent, row.Session, row.Project, row.Timestamp, row.Chars); err != nil {
			_ = appender.Close()
			return err
		}
	}
	if err := appender.Flush(); err != nil {
		_ = appender.Close()
		return err
	}
	return appender.Close()
}

func appendMatches(conn driver.Conn, rows []matchRow) error {
	if len(rows) == 0 {
		return nil
	}
	appender, err := duckdb.NewAppenderFromConn(conn, "", "matches")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := appender.AppendRow(row.MessageID, row.SourceKey, row.Agent, row.Session, row.Project, row.Timestamp, row.Word, row.Group); err != nil {
			_ = appender.Close()
			return err
		}
	}
	if err := appender.Flush(); err != nil {
		_ = appender.Close()
		return err
	}
	return appender.Close()
}

func queryAgents(ctx context.Context, db *sql.DB) ([]AgentRow, error) {
	rows, err := db.QueryContext(ctx, `
		WITH message_counts AS (
			SELECT agent, count(*) AS messages, count(DISTINCT session) AS sessions
			FROM scoped_messages
			GROUP BY agent
		),
		match_counts AS (
			SELECT agent, count(*) AS swears
			FROM scoped_matches
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
		SELECT group_name, count(*) AS swears
		FROM scoped_matches
		GROUP BY group_name
		ORDER BY swears DESC, group_name ASC
		LIMIT ?
	`, TopWords)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WordRow
	for rows.Next() {
		var row WordRow
		if err := rows.Scan(&row.Group, &row.Count); err != nil {
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
		FROM scoped_matches
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
			FROM scoped_messages
			WHERE session <> ''
			GROUP BY agent, session
		),
		match_counts AS (
			SELECT agent, session, count(*) AS swears
			FROM scoped_matches
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

func indexPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "swearjar", "index.duckdb"), nil
}

func sourceKey(src agent.Source) string {
	return src.Agent + "\x00" + src.Path
}

func messageSourceKey(msg agent.Message, id int64) string {
	if msg.Source.Path != "" {
		return sourceKey(msg.Source)
	}
	return fmt.Sprintf("inline:%d", id)
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func Scan(ctx context.Context, adapters []agent.Adapter, opts agent.Options, scope string) (Report, error) {
	return ScanWithProgress(ctx, adapters, opts, scope, nil)
}

func ScanWithProgress(ctx context.Context, adapters []agent.Adapter, opts agent.Options, scope string, progress ProgressFunc) (Report, error) {
	store, err := NewIndexed(ctx, opts.Since)
	if err != nil {
		return Report{}, err
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var progressMu sync.Mutex
	emit := func(p Progress) {
		if progress == nil {
			return
		}
		progressMu.Lock()
		progress(p)
		progressMu.Unlock()
	}

	messages := make(chan agent.Message, 2048)
	detected := make(chan detectedMessage, 2048)
	adapterErr := make(chan error, 1)
	workerErr := make(chan error, 1)

	adapterGroup, adapterCtx := errgroup.WithContext(ctx)
	var adaptersDone atomic.Int64
	scanOpts := opts
	scanOpts.Since = nil
	scanOpts.SourceHook = store
	for i, adapter := range adapters {
		index := i + 1
		adapter := adapter
		adapterGroup.Go(func() error {
			emit(Progress{Kind: ProgressAdapterStart, Agent: adapter.Name(), AdapterIndex: index, AdapterTotal: len(adapters), AdaptersDone: adaptersDone.Load()})
			err := adapter.VisitMessages(adapterCtx, scanOpts, func(msg agent.Message) error {
				select {
				case <-adapterCtx.Done():
					return adapterCtx.Err()
				case messages <- msg:
					return nil
				}
			})
			if err != nil {
				return fmt.Errorf("%s: %w", adapter.Name(), err)
			}
			done := adaptersDone.Add(1)
			emit(Progress{Kind: ProgressAdapterDone, Agent: adapter.Name(), AdapterIndex: index, AdapterTotal: len(adapters), AdaptersDone: done})
			return nil
		})
	}
	go func() {
		adapterErr <- adapterGroup.Wait()
		close(messages)
	}()

	workerGroup, workerCtx := errgroup.WithContext(ctx)
	workerCount := max(2, min(runtime.GOMAXPROCS(0), 8))
	for range workerCount {
		workerGroup.Go(func() error {
			d := detector.New()
			for msg := range messages {
				result := d.Detect(msg.Text)
				select {
				case <-workerCtx.Done():
					return workerCtx.Err()
				case detected <- detectedMessage{Message: msg, Result: result}:
				}
			}
			return nil
		})
	}
	go func() {
		workerErr <- workerGroup.Wait()
		close(detected)
	}()

	stats := newProgressStats()
	var insertErr error
	for item := range detected {
		if insertErr != nil {
			continue
		}
		if err := store.Insert(ctx, item.Message, item.Result); err != nil {
			insertErr = err
			cancel()
			continue
		}
		emit(stats.add(item.Message, item.Result, adaptersDone.Load(), len(adapters)))
	}

	if err := <-workerErr; err != nil && !errors.Is(err, context.Canceled) {
		return Report{}, err
	}
	if err := <-adapterErr; err != nil && !errors.Is(err, context.Canceled) {
		return Report{}, err
	}
	if insertErr != nil {
		return Report{}, insertErr
	}
	return store.Report(ctx, scope)
}

type detectedMessage struct {
	Message agent.Message
	Result  detector.Result
}

type progressStats struct {
	Messages int64
	Swears   int64
	Agents   map[string]*agentProgress
	LastWord string
}

type agentProgress struct {
	Messages int64
	Swears   int64
}

func newProgressStats() *progressStats {
	return &progressStats{Agents: map[string]*agentProgress{}}
}

func (s *progressStats) add(msg agent.Message, result detector.Result, adaptersDone int64, adapterTotal int) Progress {
	s.Messages++
	s.Swears += int64(result.Count)
	agentStats := s.Agents[msg.Agent]
	if agentStats == nil {
		agentStats = &agentProgress{}
		s.Agents[msg.Agent] = agentStats
	}
	agentStats.Messages++
	agentStats.Swears += int64(result.Count)
	if len(result.Matches) > 0 {
		s.LastWord = result.Matches[0].Group
	}
	return Progress{
		Kind:          ProgressMessage,
		Agent:         msg.Agent,
		AdapterTotal:  adapterTotal,
		AdaptersDone:  adaptersDone,
		Messages:      s.Messages,
		Swears:        s.Swears,
		AgentMessages: agentStats.Messages,
		AgentSwears:   agentStats.Swears,
		LastWord:      s.LastWord,
	}
}
