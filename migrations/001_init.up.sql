CREATE TABLE jobs (
    id TEXT PRIMARY KEY,

    original_filename TEXT NOT NULL,
    source_path TEXT NOT NULL,

    size_bytes INTEGER NOT NULL CHECK (size_bytes > 0),
    duration_seconds REAL NOT NULL
        CHECK (duration_seconds > 0 AND duration_seconds <= 600),

    status TEXT NOT NULL CHECK (
        status IN (
            'queued',
            'preparing',
            'transcribing',
            'completed',
            'failed',
            'cancelled'
        )
    ),

    progress REAL CHECK (
        progress IS NULL OR (progress >= 0 AND progress <= 1)
    ),

    error_code TEXT,
    error_message TEXT,

    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    finished_at TEXT,
    removed_at TEXT,

    CHECK (
        (status IN ('completed', 'failed', 'cancelled') AND finished_at IS NOT NULL)
        OR
        (status IN ('queued', 'preparing', 'transcribing') AND finished_at IS NULL)
    ),
    CHECK (removed_at IS NULL OR status IN ('completed', 'failed', 'cancelled'))
);

CREATE INDEX idx_jobs_status ON jobs(status);


CREATE INDEX idx_jobs_cleanup ON jobs(finished_at)
    WHERE removed_at IS NULL AND status IN ('completed', 'failed', 'cancelled');
