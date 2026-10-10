package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type RosterSyncResult struct {
	Leagues  int64
	Clubs    int64
	Detached int64
	Renamed  []string
	Unmerged []string
}

type clubRename struct {
	id, from, to string
}

func (s *Store) SyncRoster(ctx context.Context) (RosterSyncResult, error) {
	return s.syncRoster(ctx, clubRoster)
}

func (s *Store) syncRoster(ctx context.Context, r *roster) (RosterSyncResult, error) {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return RosterSyncResult{}, err
	}
	defer conn.Release()

	var result RosterSyncResult
	renames, unmerged, err := r.planRenames(ctx, conn.Conn())
	if err != nil {
		return RosterSyncResult{}, fmt.Errorf("roster sync: plan renames: %w", err)
	}
	for _, c := range unmerged {
		result.Unmerged = append(result.Unmerged, fmt.Sprintf("%q duplicates %q", c.from, c.to))
	}

	statements := r.seedStatements()
	if len(renames) > 0 {
		statements = append([]string{renameClubsSQL(renames)}, statements...)
	}
	results, err := conn.Conn().PgConn().Exec(ctx, strings.Join(statements, "\n\n")).ReadAll()
	if err != nil {
		return RosterSyncResult{}, fmt.Errorf("roster sync: %w", err)
	}
	if len(results) != len(statements) {
		return RosterSyncResult{}, fmt.Errorf("roster sync: %d results for %d statements", len(results), len(statements))
	}

	if len(renames) > 0 {
		renamedIDs := map[string]bool{}
		for _, row := range results[0].Rows {
			renamedIDs[string(row[0])] = true
		}
		for _, c := range renames {
			if renamedIDs[c.id] {
				result.Renamed = append(result.Renamed, fmt.Sprintf("%q -> %q", c.from, c.to))
			} else {
				result.Unmerged = append(result.Unmerged, fmt.Sprintf("%q duplicates %q", c.from, c.to))
			}
		}
		results = results[1:]
	}
	result.Leagues = results[0].CommandTag.RowsAffected()
	result.Clubs = results[1].CommandTag.RowsAffected()
	result.Detached = results[2].CommandTag.RowsAffected()
	return result, nil
}

func (r *roster) planRenames(ctx context.Context, conn *pgx.Conn) (renames, unmerged []clubRename, err error) {
	rows, err := conn.Query(ctx, `SELECT id::text, name FROM clubs ORDER BY created_at, id`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return nil, nil, err
	}
	type club struct{ id, name string }
	var clubs []club
	for rows.Next() {
		var c club
		if err := rows.Scan(&c.id, &c.name); err != nil {
			rows.Close()
			return nil, nil, err
		}
		clubs = append(clubs, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	holder := make(map[string]string, len(clubs))
	for _, c := range clubs {
		holder[strings.ToLower(c.name)] = c.id
	}
	for _, c := range clubs {
		entry, known := r.lookup(c.name)
		if !known || entry.Name == c.name {
			continue
		}
		rename := clubRename{id: c.id, from: c.name, to: entry.Name}
		target := strings.ToLower(entry.Name)
		if id, taken := holder[target]; taken && id != c.id {
			unmerged = append(unmerged, rename)
			continue
		}
		holder[target] = c.id
		renames = append(renames, rename)
	}
	return renames, unmerged, nil
}

func renameClubsSQL(renames []clubRename) string {
	var b strings.Builder
	b.WriteString("UPDATE clubs SET name = r.name\n")
	b.WriteString("FROM (VALUES\n")
	for i, c := range renames {
		fmt.Fprintf(&b, "    (%s::uuid, %s)%s\n", quoteSQL(c.id), quoteSQL(c.to), listSeparator(i, len(renames)))
	}
	b.WriteString(") AS r(id, name)\n")
	b.WriteString("WHERE clubs.id = r.id\n")
	b.WriteString("AND NOT EXISTS (SELECT 1 FROM clubs other WHERE lower(other.name) = lower(r.name) AND other.id <> r.id)\n")
	b.WriteString("RETURNING clubs.id::text;")
	return b.String()
}

func rosterSeedSQL() string {
	return strings.Join(clubRoster.seedStatements(), "\n\n")
}

func (r *roster) seedStatements() []string {
	return []string{r.upsertLeaguesSQL(), r.upsertClubsSQL(), r.detachDepartedClubsSQL()}
}

func (r *roster) upsertLeaguesSQL() string {
	var b strings.Builder
	b.WriteString("INSERT INTO leagues (name, short_name) VALUES\n")
	for i, league := range r.leagues {
		fmt.Fprintf(&b, "    (%s, %s)%s\n", quoteSQL(league.Name), quoteSQL(league.ShortName), listSeparator(i, len(r.leagues)))
	}
	b.WriteString("ON CONFLICT (lower(name)) DO UPDATE\n")
	b.WriteString("SET name = EXCLUDED.name,\n")
	b.WriteString("    short_name = EXCLUDED.short_name;")
	return b.String()
}

func (r *roster) upsertClubsSQL() string {
	entries := r.entries()
	var b strings.Builder
	b.WriteString("INSERT INTO clubs (name, short_name, crest_url, league_id)\n")
	b.WriteString("SELECT c.name, c.short_name, NULLIF(c.crest_url, ''), l.id\n")
	b.WriteString("FROM (VALUES\n")
	for i, entry := range entries {
		fmt.Fprintf(&b, "    (%s, %s, %s, %s)%s\n",
			quoteSQL(entry.Name), quoteSQL(entry.ShortName), quoteSQL(entry.CrestURL), quoteSQL(entry.League),
			listSeparator(i, len(entries)))
	}
	b.WriteString(") AS c(name, short_name, crest_url, league)\n")
	b.WriteString("JOIN leagues l ON lower(l.name) = lower(c.league)\n")
	b.WriteString("ON CONFLICT (lower(name)) DO UPDATE\n")
	b.WriteString("SET name = EXCLUDED.name,\n")
	b.WriteString("    short_name = EXCLUDED.short_name,\n")
	b.WriteString("    crest_url = COALESCE(EXCLUDED.crest_url, clubs.crest_url),\n")
	b.WriteString("    league_id = EXCLUDED.league_id;")
	return b.String()
}

func (r *roster) detachDepartedClubsSQL() string {
	entries := r.entries()
	var b strings.Builder
	b.WriteString("UPDATE clubs SET league_id = NULL\n")
	b.WriteString("WHERE league_id IN (\n")
	b.WriteString("    SELECT l.id FROM leagues l\n")
	b.WriteString("    JOIN (VALUES\n")
	for i, league := range r.leagues {
		fmt.Fprintf(&b, "        (%s)%s\n", quoteSQL(league.Name), listSeparator(i, len(r.leagues)))
	}
	b.WriteString("    ) AS r(name) ON lower(l.name) = lower(r.name)\n")
	b.WriteString(")\n")
	b.WriteString("AND lower(name) NOT IN (\n")
	b.WriteString("    SELECT lower(r.name) FROM (VALUES\n")
	for i, entry := range entries {
		fmt.Fprintf(&b, "        (%s)%s\n", quoteSQL(entry.Name), listSeparator(i, len(entries)))
	}
	b.WriteString("    ) AS r(name)\n")
	b.WriteString(");")
	return b.String()
}

func (r *roster) entries() []rosterEntry {
	var entries []rosterEntry
	for _, league := range r.leagues {
		for _, club := range league.Clubs {
			entries = append(entries, rosterEntry{rosterClub: club, League: league.Name})
		}
	}
	return entries
}

func listSeparator(i, n int) string {
	if i == n-1 {
		return ""
	}
	return ","
}

func quoteSQL(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
