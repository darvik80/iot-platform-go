-- +migrate Up

ALTER TABLE products ADD COLUMN methods_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE products ADD COLUMN metrics_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE products ADD COLUMN attributes_json TEXT NOT NULL DEFAULT '[]';
