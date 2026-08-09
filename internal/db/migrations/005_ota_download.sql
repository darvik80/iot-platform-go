-- +migrate Up

ALTER TABLE ota_firmwares ADD COLUMN download_token TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_ota_fw_download_token ON ota_firmwares(download_token) WHERE download_token != '';
