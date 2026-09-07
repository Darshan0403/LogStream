-- init.sql

-- 1. Enable Trigram extension for text similarity and ILIKE search performance
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 2. Core logs table with native partitioning
CREATE TABLE logs (
    id BIGSERIAL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    level VARCHAR(10) NOT NULL,
    service VARCHAR(100) NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);
-- Note: search is trigram/ILIKE on `message` (idx_logs_message_trgm). A previous
-- tsvector generated column was dropped — it only added write amplification.
-- Existing databases: ALTER TABLE logs DROP COLUMN IF EXISTS search_vector;

-- 3. Default partition — catch-all for timestamps outside any weekly partition.
-- The application provisions rolling weekly partitions at startup and every 6h
-- (storage.EnsurePartitions), so on a normal deployment new rows land in a
-- weekly partition and this stays empty.
CREATE TABLE logs_default PARTITION OF logs DEFAULT;

-- 4. Indexes for fast retrieval
-- Note: Creating indexes on the parent table automatically cascades them to all partitions
CREATE INDEX idx_logs_timestamp    ON logs USING BRIN (timestamp);
CREATE INDEX idx_logs_service      ON logs USING BTREE (service);
CREATE INDEX idx_logs_level        ON logs USING BTREE (level);
CREATE INDEX idx_logs_metadata     ON logs USING GIN (metadata);
CREATE INDEX idx_logs_message_trgm ON logs USING GIN (message gin_trgm_ops);

-- 5. Alert Rules
CREATE TABLE alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    pattern VARCHAR(500) NOT NULL,
    level_filter VARCHAR(10),
    service_filter VARCHAR(100),
    cooldown_minutes INT DEFAULT 5,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 6. Alert History (Cascades on rule deletion)
CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID REFERENCES alert_rules(id) ON DELETE CASCADE,
    log_id BIGINT,
    log_timestamp TIMESTAMPTZ,
    fired_at TIMESTAMPTZ DEFAULT NOW(),
    hit_count INT DEFAULT 1,
    -- Foreign key to partitioned table requires matching the partition key
    FOREIGN KEY (log_id, log_timestamp) REFERENCES logs(id, timestamp) ON DELETE CASCADE
);

-- 7. One alert row per rule (deduplication).
-- Required by CreateAlert's `INSERT ... ON CONFLICT (rule_id) DO UPDATE`, which
-- errors without a unique index/constraint on the conflict target.
CREATE UNIQUE INDEX idx_alerts_rule_id ON alerts (rule_id);