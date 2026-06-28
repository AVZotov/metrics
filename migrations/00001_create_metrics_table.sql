-- +goose Up
CREATE TABLE metrics
(
    id    TEXT NOT NULL,
    mtype TEXT NOT NULL,
    delta BIGINT,
    value DOUBLE PRECISION,
    hash  TEXT,
    PRIMARY KEY (id, mtype)
);

-- +goose Down
DROP TABLE metrics;
