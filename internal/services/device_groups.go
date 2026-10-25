package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cogitor-iot-platform/internal/models"
)

var (
	ErrGroupNotFound = errors.New("device group not found")
	ErrGroupInvalid  = errors.New("invalid device group")
)

func (p *Platform) ListDeviceGroups(accountID int64) ([]models.DeviceGroup, error) {
	if accountID <= 0 {
		return nil, ErrGroupNotFound
	}
	rows, err := p.DB.Query(`SELECT id, account_id, name, description, scope_type, scope_filter, created_at, updated_at FROM device_groups WHERE account_id=? ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []models.DeviceGroup
	for rows.Next() {
		var g models.DeviceGroup
		if err := rows.Scan(&g.ID, &g.AccountID, &g.Name, &g.Description, &g.ScopeType, &g.ScopeFilter, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	if groups == nil {
		groups = []models.DeviceGroup{}
	}
	return groups, rows.Err()
}

func (p *Platform) GetDeviceGroup(accountID, groupID int64) (*models.DeviceGroup, error) {
	if accountID <= 0 || groupID <= 0 {
		return nil, ErrGroupNotFound
	}
	var g models.DeviceGroup
	err := p.DB.QueryRow(`SELECT id, account_id, name, description, scope_type, scope_filter, created_at, updated_at FROM device_groups WHERE id=? AND account_id=?`, groupID, accountID).Scan(&g.ID, &g.AccountID, &g.Name, &g.Description, &g.ScopeType, &g.ScopeFilter, &g.CreatedAt, &g.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrGroupNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (p *Platform) SaveDeviceGroup(accountID, groupID int64, in models.DeviceGroupInput) (*models.DeviceGroup, error) {
	if accountID <= 0 {
		return nil, ErrGroupNotFound
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		return nil, fmt.Errorf("%w: name must be 1-100 characters", ErrGroupInvalid)
	}
	if len(in.Description) > 500 {
		return nil, fmt.Errorf("%w: description too long", ErrGroupInvalid)
	}
	validScopes := map[string]bool{"manual": true, "product": true, "tag": true}
	if !validScopes[in.ScopeType] {
		return nil, fmt.Errorf("%w: scope_type must be manual, product, or tag", ErrGroupInvalid)
	}
	if in.ScopeFilter == "" {
		in.ScopeFilter = "{}"
	}
	var filter map[string]any
	if err := json.Unmarshal([]byte(in.ScopeFilter), &filter); err != nil {
		return nil, fmt.Errorf("%w: invalid scope_filter JSON", ErrGroupInvalid)
	}
	now := time.Now().Unix()
	if groupID <= 0 {
		res, err := p.DB.Exec(`INSERT INTO device_groups (account_id, name, description, scope_type, scope_filter, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			accountID, in.Name, in.Description, in.ScopeType, in.ScopeFilter, now, now)
		if err != nil {
			return nil, err
		}
		groupID, _ = res.LastInsertId()
	} else {
		var existing int64
		if err := p.DB.QueryRow(`SELECT id FROM device_groups WHERE id=? AND account_id=?`, groupID, accountID).Scan(&existing); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrGroupNotFound
			}
			return nil, err
		}
		if _, err := p.DB.Exec(`UPDATE device_groups SET name=?, description=?, scope_type=?, scope_filter=?, updated_at=? WHERE id=?`,
			in.Name, in.Description, in.ScopeType, in.ScopeFilter, now, groupID); err != nil {
			return nil, err
		}
	}
	return p.GetDeviceGroup(accountID, groupID)
}

func (p *Platform) DeleteDeviceGroup(accountID, groupID int64) error {
	if accountID <= 0 || groupID <= 0 {
		return ErrGroupNotFound
	}
	res, err := p.DB.Exec(`DELETE FROM device_groups WHERE id=? AND account_id=?`, groupID, accountID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrGroupNotFound
	}
	return nil
}

func (p *Platform) GetDeviceGroupMembers(accountID, groupID int64) ([]int64, error) {
	g, err := p.GetDeviceGroup(accountID, groupID)
	if err != nil {
		return nil, err
	}
	if g.ScopeType != "manual" {
		return p.ResolveScope(models.Scope{Type: models.ScopeType(g.ScopeType), GroupID: groupID})
	}
	rows, err := p.DB.Query(`SELECT device_id FROM device_group_members WHERE group_id=? ORDER BY device_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []int64{}
	}
	return ids, rows.Err()
}

func (p *Platform) SetDeviceGroupMembers(accountID, groupID int64, deviceIDs []int64) error {
	g, err := p.GetDeviceGroup(accountID, groupID)
	if err != nil {
		return err
	}
	if g.ScopeType != "manual" {
		return fmt.Errorf("%w: cannot set members for non-manual group", ErrGroupInvalid)
	}
	tx, err := p.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM device_group_members WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, deviceID := range deviceIDs {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM devices d JOIN products pr ON pr.id=d.product_pk WHERE d.id=? AND pr.account_id=?`, deviceID, accountID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("%w: device %d not found in account", ErrGroupInvalid, deviceID)
		}
		if _, err := tx.Exec(`INSERT INTO device_group_members (group_id, device_id) VALUES (?, ?)`, groupID, deviceID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Platform) ResolveScope(scope models.Scope) ([]int64, error) {
	switch scope.Type {
	case models.ScopeDevice:
		if scope.DeviceID <= 0 {
			return nil, fmt.Errorf("invalid device_id")
		}
		return []int64{scope.DeviceID}, nil
	case models.ScopeGroup:
		if scope.GroupID <= 0 {
			return nil, fmt.Errorf("invalid group_id")
		}
		rows, err := p.DB.Query(`SELECT device_id FROM device_group_members WHERE group_id=?`, scope.GroupID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	case models.ScopeProduct:
		if scope.ProductID <= 0 {
			return nil, fmt.Errorf("invalid product_id")
		}
		rows, err := p.DB.Query(`SELECT id FROM devices WHERE product_pk=?`, scope.ProductID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	case models.ScopeTag:
		if scope.Tag == "" {
			return nil, fmt.Errorf("invalid tag")
		}
		rows, err := p.DB.Query(`SELECT id FROM devices WHERE json_extract(tags, '$') LIKE ?`, `%"`+scope.Tag+`"%`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	default:
		return nil, fmt.Errorf("unknown scope type: %s", scope.Type)
	}
}
