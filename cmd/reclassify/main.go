// Command reclassify is a one-off clean-up for rumours stored under the old
// extraction gate ("confidence > 0", before the is_transfer_rumour flag and
// the Jev gate). It re-judges every article linked to each rumour with the
// Jev classifier alone — Claude is never called — and marks for deletion
// every rumour whose articles ALL fall below JEV_MIN_PROBABILITY.
//
// It is a dry run unless -apply is given: the default prints the report and
// what a deletion would remove, and changes nothing. With -apply, the
// deletion happens in one transaction after every article has been
// classified; any Jev error aborts the run before anything is deleted.
//
//	go run ./cmd/reclassify                # dry run
//	go run ./cmd/reclassify -apply         # delete
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// defaultBefore is the day after the old-gate corpus was frozen
// (2026-09-11), so every rumour stored under "confidence > 0" is in scope
// and none stored by the Jev-gated extractor is.
const defaultBefore = "2026-09-12"

func main() {
	apply := flag.Bool("apply", false, "delete the rumours the report marks DELETE (default: dry run)")
	before := flag.String("before", defaultBefore, "only rumours created before this date (YYYY-MM-DD, UTC)")
	flag.Parse()

	if err := run(*apply, *before); err != nil {
		slog.Error("reclassify", "error", err)
		os.Exit(1)
	}
}

func run(apply bool, beforeFlag string) error {
	cutoff, err := time.Parse(time.DateOnly, beforeFlag)
	if err != nil {
		return fmt.Errorf("-before: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.TypesafeAPIKey == "" {
		return fmt.Errorf("TYPESAFE_API_KEY is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	s := store.New(pool)

	rumours, err := s.ListRumoursForReclassify(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("list rumours: %w", err)
	}

	classifier := extract.NewJevClassifierFromConfig(cfg)
	slog.Info("reclassify: starting", "rumours", len(rumours), "before", beforeFlag,
		"jev_model", cfg.JevModel, "jev_min_probability", cfg.JevMinProbability, "apply", apply)

	// An article can sit on several rumours' timelines; classify it once.
	probabilities := map[uuid.UUID]float64{}
	var toDelete []uuid.UUID
	var kept, noArticles int
	for _, r := range rumours {
		if len(r.Articles) == 0 {
			// Nothing to re-judge. Kept rather than deleted: "all of zero
			// articles are below the threshold" is not evidence.
			noArticles++
			fmt.Printf("rumour %s  %s -> %s  verdict=KEEP (no linked articles)\n\n", r.ID, r.PlayerName, r.ToClubName)
			continue
		}

		allBelow := true
		lines := make([]string, 0, len(r.Articles))
		for _, a := range r.Articles {
			p, ok := probabilities[a.ID]
			if !ok {
				body := ""
				if a.Content != nil {
					body = *a.Content
				}
				c, err := classifier.Classify(ctx, a.Title, body)
				if err != nil {
					return fmt.Errorf("classify article %s (rumour %s), nothing deleted: %w", a.ID, r.ID, err)
				}
				p = c.Probability
				probabilities[a.ID] = p
			}
			if p >= cfg.JevMinProbability {
				allBelow = false
			}
			lines = append(lines, fmt.Sprintf("  %.3f  %s", p, a.Title))
		}

		verdict := "KEEP"
		if allBelow {
			verdict = "DELETE"
			toDelete = append(toDelete, r.ID)
		} else {
			kept++
		}
		fmt.Printf("rumour %s  %s -> %s  verdict=%s\n", r.ID, r.PlayerName, r.ToClubName, verdict)
		for _, l := range lines {
			fmt.Println(l)
		}
		fmt.Println()
	}

	fmt.Printf("summary: %d rumours, %d keep, %d keep (no linked articles), %d delete; %d articles classified\n",
		len(rumours), kept, noArticles, len(toDelete), len(probabilities))
	if len(toDelete) == 0 {
		return nil
	}

	if !apply {
		d, err := s.CountRumourDeletion(ctx, toDelete)
		if err != nil {
			return fmt.Errorf("count deletion: %w", err)
		}
		fmt.Printf("dry run: -apply would delete %d rumours, cascading %d rumour_events, %d comments, %d likes (articles are kept)\n",
			d.Rumours, d.Events, d.Comments, d.Likes)
		return nil
	}

	d, err := s.DeleteRumours(ctx, toDelete)
	if err != nil {
		return fmt.Errorf("delete rumours: %w", err)
	}
	fmt.Printf("applied: deleted %d rumours, cascading %d rumour_events, %d comments, %d likes (articles are kept)\n",
		d.Rumours, d.Events, d.Comments, d.Likes)
	return nil
}
