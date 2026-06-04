-- init.sql

-- 1. Core logs table with native partitioning
CREATE TABLE logs (
    id BIGSERIAL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    level VARCHAR(10) NOT NULL,
    service VARCHAR(100) NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    search_vector TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('english', service || ' ' || message)
    ) STORED,
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

-- 2. Create the default partition (Catches everything until specific weekly partitions are made)
CREATE TABLE logs_default PARTITION OF logs DEFAULT;

-- 3. Indexes for fast retrieval
CREATE INDEX idx_logs_timestamp ON logs USING BRIN (timestamp);
CREATE INDEX idx_logs_service   ON logs (service);
CREATE INDEX idx_logs_level     ON logs (level);
CREATE INDEX idx_logs_search    ON logs USING GIN (search_vector);
CREATE INDEX idx_logs_metadata  ON logs USING GIN (metadata);

-- 4. Alert Rules
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

-- 5. Alert History (Cascades on rule deletion)
CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID REFERENCES alert_rules(id) ON DELETE CASCADE,
    log_id BIGINT,
    log_timestamp TIMESTAMPTZ,
    fired_at TIMESTAMPTZ DEFAULT NOW(),
    -- Foreign key to partitioned table requires matching the partition key
    FOREIGN KEY (log_id, log_timestamp) REFERENCES logs(id, timestamp) ON DELETE CASCADE
);