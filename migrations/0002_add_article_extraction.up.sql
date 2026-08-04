-- Raw LLM extraction output per article, per the internal/extract.Result
-- shape. NULL means extraction was never attempted, failed, or found
-- nothing usable (the model returns confidence 0 in that case, per
-- SystemPrompt). Milestone 1.4 consumes this to upsert rumours.
ALTER TABLE articles ADD COLUMN extraction JSONB;
