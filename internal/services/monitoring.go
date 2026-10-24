package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

var (
	ErrMonitorInvalid  = errors.New("invalid monitoring configuration")
	ErrMonitorNotFound = errors.New("monitoring resource not found")
	ErrMonitorConflict = errors.New("dashboard changed; reload before saving")
	monitorColor       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

const monitorMaxWindow = 31 * 24 * 3600
const monitorMaxPoints = 240

var monitorAggregates = map[string]string{"avg": "AVG", "min": "MIN", "max": "MAX", "sum": "SUM", "last": ""}
var monitorTypes = map[string]struct{ scalar bool }{
	"line": {}, "bar": {}, "stat": {scalar: true}, "gauge": {scalar: true}, "table": {},
}

func monitorInvalid(message string) error {
	return fmt.Errorf("%w: %s", ErrMonitorInvalid, message)
}

func (p *Platform) monitorDevice(accountID, deviceID int64) error {
	if accountID <= 0 || deviceID <= 0 {
		return ErrMonitorNotFound
	}
	var found int
	if err := p.DB.QueryRow(`SELECT 1 FROM devices d JOIN products pr ON pr.id=d.product_pk WHERE d.id=? AND pr.account_id=?`, deviceID, accountID).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMonitorNotFound
		}
		return err
	}
	return nil
}

func (p *Platform) MonitoringMetrics(accountID, deviceID int64) ([]models.MonitoringMetric, error) {
	if err := p.monitorDevice(accountID, deviceID); err != nil {
		return nil, err
	}
	rows, err := p.DB.Query(`SELECT key, CASE WHEN MAX(CASE WHEN value_text IS NOT NULL THEN 1 ELSE 0 END)=1 THEN 'text' WHEN MAX(CASE WHEN value_num IS NOT NULL THEN 1 ELSE 0 END)=1 THEN 'number' ELSE 'boolean' END, MAX(ingested_at) FROM telemetry_samples WHERE account_id=? AND device_pk=? GROUP BY key ORDER BY key LIMIT 1000`, accountID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MonitoringMetric{}
	for rows.Next() {
		var m models.MonitoringMetric
		if err := rows.Scan(&m.Key, &m.Kind, &m.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (p *Platform) validateMonitorWidget(accountID int64, w *models.MonitoringWidget) error {
	w.Title = strings.TrimSpace(w.Title)
	_, validType := monitorTypes[w.Type]
	if len(w.ID) == 0 || len(w.ID) > 64 || len(w.Title) == 0 || len(w.Title) > 100 || !validType {
		return monitorInvalid("widget needs an ID, title and supported type")
	}
	if _, ok := monitorAggregates[w.Aggregation]; !ok {
		return monitorInvalid("unsupported aggregation")
	}
	if w.WindowSeconds < 60 || w.WindowSeconds > monitorMaxWindow {
		return monitorInvalid("window must be between one minute and 31 days")
	}
	if w.Width != 6 && w.Width != 12 {
		return monitorInvalid("widget width must be 6 or 12")
	}
	if math.IsNaN(w.Min) || math.IsNaN(w.Max) || math.IsInf(w.Min, 0) || math.IsInf(w.Max, 0) || (w.Type == "gauge" && w.Min >= w.Max) {
		return monitorInvalid("invalid gauge range")
	}
	if len(w.Sources) == 0 || len(w.Sources) > 8 {
		return monitorInvalid("select between one and eight metric sources")
	}
	seen := map[string]bool{}
	for i := range w.Sources {
		s := &w.Sources[i]
		s.Label = strings.TrimSpace(s.Label)
		if s.Key == "" || len(s.Key) > 128 || len(s.Label) > 100 {
			return monitorInvalid("invalid metric key or label")
		}
		if s.Color == "" {
			s.Color = "#3d9cf0"
		}
		if !monitorColor.MatchString(s.Color) {
			return monitorInvalid("color must be a six-digit hex value")
		}
		identity := fmt.Sprintf("%d/%s", s.DeviceID, s.Key)
		if seen[identity] {
			return monitorInvalid("duplicate metric source")
		}
		seen[identity] = true
		if err := p.monitorDevice(accountID, s.DeviceID); err != nil {
			return err
		}
	}
	return nil
}

func (p *Platform) SaveMonitoringDashboard(accountID, id int64, in models.MonitoringDashboard) (*models.MonitoringDashboard, error) {
	if accountID <= 0 {
		return nil, ErrMonitorNotFound
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || len(in.Widgets) > 24 {
		return nil, monitorInvalid("dashboard needs a name and at most 24 widgets")
	}
	seen := map[string]bool{}
	for i := range in.Widgets {
		if err := p.validateMonitorWidget(accountID, &in.Widgets[i]); err != nil {
			return nil, err
		}
		if seen[in.Widgets[i].ID] {
			return nil, monitorInvalid("duplicate widget ID")
		}
		seen[in.Widgets[i].ID] = true
	}
	tx, err := p.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := db.NowMS()
	if id == 0 {
		if p.DB.Dialect == db.DialectPostgres {
			var locked int64
			if err := tx.QueryRow(`SELECT id FROM accounts WHERE id=? FOR UPDATE`, accountID).Scan(&locked); err != nil {
				return nil, err
			}
		}
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM monitoring_dashboards WHERE account_id=?`, accountID).Scan(&count); err != nil {
			return nil, err
		}
		if count >= 50 {
			return nil, monitorInvalid("dashboard limit reached")
		}
		res, err := tx.Exec(`INSERT INTO monitoring_dashboards(account_id,name,version,created_at,updated_at) VALUES(?,?,1,?,?)`, accountID, in.Name, now, now)
		if err != nil {
			return nil, err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
		in.Version = 1
	} else {
		var version int64
		if err := tx.QueryRow(`SELECT version FROM monitoring_dashboards WHERE account_id=? AND id=?`, accountID, id).Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrMonitorNotFound
			}
			return nil, err
		}
		if version != in.Version {
			return nil, ErrMonitorConflict
		}
		res, err := tx.Exec(`UPDATE monitoring_dashboards SET name=?,version=version+1,updated_at=? WHERE account_id=? AND id=? AND version=?`, in.Name, now, accountID, id, in.Version)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, ErrMonitorConflict
		}
		in.Version++
		if _, err := tx.Exec(`DELETE FROM monitoring_widgets WHERE dashboard_id=?`, id); err != nil {
			return nil, err
		}
	}
	for i, w := range in.Widgets {
		raw, err := json.Marshal(w)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO monitoring_widgets(dashboard_id,position,config_json) VALUES(?,?,?)`, id, i, string(raw)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	in.ID = id
	in.AccountID = accountID
	if in.Widgets == nil {
		in.Widgets = []models.MonitoringWidget{}
	}
	return &in, nil
}

func (p *Platform) GetMonitoringDashboard(accountID, id int64) (*models.MonitoringDashboard, error) {
	d := models.MonitoringDashboard{Widgets: []models.MonitoringWidget{}}
	rows, err := p.DB.Query(`SELECT d.id,d.account_id,d.name,d.version,w.config_json FROM monitoring_dashboards d LEFT JOIN monitoring_widgets w ON w.dashboard_id=d.id WHERE d.account_id=? AND d.id=? ORDER BY w.position`, accountID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw sql.NullString
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Name, &d.Version, &raw); err != nil {
			return nil, err
		}
		if raw.Valid {
			var w models.MonitoringWidget
			if err := json.Unmarshal([]byte(raw.String), &w); err != nil {
				return nil, err
			}
			d.Widgets = append(d.Widgets, w)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if d.ID == 0 {
		return nil, ErrMonitorNotFound
	}
	return &d, nil
}

func (p *Platform) ListMonitoringDashboards(accountID int64) ([]models.MonitoringDashboard, error) {
	rows, err := p.DB.Query(`SELECT id,account_id,name,version FROM monitoring_dashboards WHERE account_id=? ORDER BY id LIMIT 50`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.MonitoringDashboard{}
	for rows.Next() {
		var d models.MonitoringDashboard
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Name, &d.Version); err != nil {
			return nil, err
		}
		d.Widgets = []models.MonitoringWidget{}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *Platform) DeleteMonitoringDashboard(accountID, id int64) error {
	res, err := p.DB.Exec(`DELETE FROM monitoring_dashboards WHERE account_id=? AND id=?`, accountID, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMonitorNotFound
	}
	return nil
}

func (p *Platform) QueryMonitoring(accountID int64, in models.MonitoringQuery) (*models.MonitoringData, error) {
	if err := p.validateMonitorWidget(accountID, &in.Widget); err != nil {
		return nil, err
	}
	now := db.NowMS()
	if in.To == 0 {
		in.To = now
	}
	if in.From == 0 {
		in.From = in.To - int64(in.Widget.WindowSeconds)*1000
	}
	if in.From < 0 || in.From >= in.To || in.To > now+60000 || in.To-in.From > int64(monitorMaxWindow)*1000 {
		return nil, monitorInvalid("invalid query time range")
	}
	out := &models.MonitoringData{From: in.From, To: in.To, Series: []models.MonitoringSeries{}}
	bucket := (in.To-in.From)/monitorMaxPoints + 1
	if monitorTypes[in.Widget.Type].scalar {
		bucket = in.To - in.From + 1
	}
	for _, s := range in.Widget.Sources {
		series := models.MonitoringSeries{DeviceID: s.DeviceID, Key: s.Key, Label: s.Label, Color: s.Color, Points: []models.MonitoringPoint{}}
		if series.Label == "" {
			series.Label = s.Key
		}
		var rows *sql.Rows
		var err error
		if in.Widget.Aggregation == "last" {
			rows, err = p.DB.Query(`SELECT ts,value_num,value_bool,value_text FROM (SELECT ts,value_num,value_bool,value_text,ROW_NUMBER() OVER (PARTITION BY CAST((ts-?)/? AS BIGINT) ORDER BY ts DESC,id DESC) AS rn FROM telemetry_samples WHERE account_id=? AND device_pk=? AND key=? AND ts>=? AND ts<=?) samples WHERE rn=1 ORDER BY ts LIMIT 240`, in.From, bucket, accountID, s.DeviceID, s.Key, in.From, in.To)
		} else {
			aggregate := monitorAggregates[in.Widget.Aggregation]
			rows, err = p.DB.Query(`SELECT MAX(ts),`+aggregate+`(value_num),NULL,NULL FROM telemetry_samples WHERE account_id=? AND device_pk=? AND key=? AND ts>=? AND ts<=? AND value_num IS NOT NULL GROUP BY CAST((ts-?)/? AS BIGINT) ORDER BY MAX(ts) LIMIT 240`, accountID, s.DeviceID, s.Key, in.From, in.To, in.From, bucket)
		}
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ts int64
			var number sql.NullFloat64
			var boolean sql.NullInt64
			var text sql.NullString
			if err := rows.Scan(&ts, &number, &boolean, &text); err != nil {
				rows.Close()
				return nil, err
			}
			var value any
			switch {
			case number.Valid:
				if math.IsNaN(number.Float64) || math.IsInf(number.Float64, 0) {
					rows.Close()
					return nil, monitorInvalid("metric aggregate is outside the finite numeric range")
				}
				value = number.Float64
			case boolean.Valid:
				value = boolean.Int64 != 0
			case text.Valid:
				value = text.String
			}
			series.Points = append(series.Points, models.MonitoringPoint{TS: ts, Value: value})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		out.Series = append(out.Series, series)
	}
	return out, nil
}
