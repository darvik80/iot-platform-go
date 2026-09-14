package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
)

const productSelect = `SELECT id, COALESCE(account_id,0), product_id, name, description, topic_style_default,
	COALESCE(auto_ota,0), COALESCE(methods_json,'[]'), COALESCE(metrics_json,'[]'), COALESCE(attributes_json,'[]'),
	created_at, updated_at FROM products`

func scanProduct(row interface{ Scan(dest ...any) error }) (*models.Product, error) {
	var pr models.Product
	var methodsJSON, metricsJSON, attrsJSON string
	var autoOTA int
	err := row.Scan(&pr.ID, &pr.AccountID, &pr.ProductID, &pr.Name, &pr.Description, &pr.TopicStyleDefault, &autoOTA,
		&methodsJSON, &metricsJSON, &attrsJSON, &pr.CreatedAt, &pr.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("not found")
	}
	if err != nil {
		return nil, err
	}
	pr.AutoOTA = autoOTA == 1
	pr.Methods = decodeMethods(methodsJSON)
	pr.Metrics = decodeMetrics(metricsJSON)
	pr.Attributes = decodeAttributes(attrsJSON)
	MergeProductPresets(&pr)
	return &pr, nil
}

func decodeMethods(s string) []models.ProductMethod {
	var v []models.ProductMethod
	if s == "" {
		s = "[]"
	}
	_ = json.Unmarshal([]byte(s), &v)
	if v == nil {
		v = []models.ProductMethod{}
	}
	return v
}

func decodeMetrics(s string) []models.ProductMetric {
	var v []models.ProductMetric
	if s == "" {
		s = "[]"
	}
	_ = json.Unmarshal([]byte(s), &v)
	if v == nil {
		v = []models.ProductMetric{}
	}
	return v
}

func decodeAttributes(s string) []models.ProductAttribute {
	var v []models.ProductAttribute
	if s == "" {
		s = "[]"
	}
	_ = json.Unmarshal([]byte(s), &v)
	if v == nil {
		v = []models.ProductAttribute{}
	}
	return v
}

func (p *Platform) ListProducts(accountID int64) ([]models.Product, error) {
	var rows *sql.Rows
	var err error
	if accountID > 0 {
		rows, err = p.DB.Query(productSelect+` WHERE account_id=? ORDER BY name`, accountID)
	} else {
		rows, err = p.DB.Query(productSelect + ` ORDER BY name`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Product
	for rows.Next() {
		pr, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *pr)
	}
	if out == nil {
		out = []models.Product{}
	}
	return out, nil
}

func (p *Platform) GetProduct(id int64) (*models.Product, error) {
	return scanProduct(p.DB.QueryRow(productSelect+` WHERE id=?`, id))
}

// GetProductScoped returns a product only if it belongs to the given account.
// Pass accountID=0 to skip the account check (internal callers).
func (p *Platform) GetProductScoped(accountID, id int64) (*models.Product, error) {
	pr, err := scanProduct(p.DB.QueryRow(productSelect+` WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if accountID > 0 && pr.AccountID != accountID {
		return nil, errors.New("not found")
	}
	return pr, nil
}

func (p *Platform) GetProductBySlug(slug string) (*models.Product, error) {
	return scanProduct(p.DB.QueryRow(productSelect+` WHERE product_id=?`, slug))
}

type ProductInput struct {
	ProductID         string                    `json:"product_id"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	TopicStyleDefault string                    `json:"topic_style_default"`
	Methods           []models.ProductMethod    `json:"methods"`
	Metrics           []models.ProductMetric    `json:"metrics"`
	Attributes        []models.ProductAttribute `json:"attributes"`
	// SetSchema true when methods/metrics/attributes provided (including empty arrays)
	AutoOTA       *bool `json:"auto_ota"`
	SetMethods    bool  `json:"-"`
	SetMetrics    bool  `json:"-"`
	SetAttributes bool  `json:"-"`
	SetAutoOTA    bool  `json:"-"`
}

func (p *Platform) CreateProduct(accountID int64, in ProductInput) (*models.Product, error) {
	in.ProductID = strings.TrimSpace(in.ProductID)
	in.Name = strings.TrimSpace(in.Name)
	if in.ProductID == "" && in.Name != "" {
		in.ProductID = slugifyID(in.Name)
	}
	if !idRe.MatchString(in.ProductID) {
		return nil, errors.New("invalid product_id: use [a-zA-Z0-9][a-zA-Z0-9._-]{0,62} (e.g. esp32-cam)")
	}
	if in.Name == "" {
		in.Name = in.ProductID
	}
	if in.TopicStyleDefault == "" {
		in.TopicStyleDefault = protocol.StyleShort
	}
	if !protocol.ValidStyle(in.TopicStyleDefault) {
		return nil, errors.New("invalid topic_style_default")
	}
	if in.Methods == nil {
		in.Methods = DefaultESPMethods()
	} else {
		in.Methods = mergeMethods(DefaultESPMethods(), in.Methods)
	}
	if in.Metrics == nil {
		in.Metrics = DefaultESPMetrics()
	} else {
		in.Metrics = mergeMetrics(DefaultESPMetrics(), in.Metrics)
	}
	if in.Attributes == nil {
		in.Attributes = []models.ProductAttribute{}
	}
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO products(account_id, product_id, name, description, topic_style_default, methods_json, metrics_json, attributes_json, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		accountID, in.ProductID, in.Name, in.Description, in.TopicStyleDefault,
		mustJSON(in.Methods), mustJSON(in.Metrics), mustJSON(in.Attributes), now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return p.GetProduct(id)
}

func (p *Platform) UpdateProduct(id int64, in ProductInput) (*models.Product, error) {
	pr, err := p.GetProduct(id)
	if err != nil {
		return nil, err
	}
	if in.Name != "" {
		pr.Name = in.Name
	}
	pr.Description = in.Description
	if in.TopicStyleDefault != "" {
		if !protocol.ValidStyle(in.TopicStyleDefault) {
			return nil, errors.New("invalid topic_style_default")
		}
		pr.TopicStyleDefault = in.TopicStyleDefault
	}
	if in.SetMethods || in.Methods != nil {
		pr.Methods = in.Methods
		if pr.Methods == nil {
			pr.Methods = []models.ProductMethod{}
		}
		pr.Methods = mergeMethods(DefaultESPMethods(), pr.Methods)
	}
	if in.SetMetrics || in.Metrics != nil {
		pr.Metrics = in.Metrics
		if pr.Metrics == nil {
			pr.Metrics = []models.ProductMetric{}
		}
		pr.Metrics = mergeMetrics(DefaultESPMetrics(), pr.Metrics)
	}
	if in.SetAttributes || in.Attributes != nil {
		pr.Attributes = in.Attributes
		if pr.Attributes == nil {
			pr.Attributes = []models.ProductAttribute{}
		}
	}
	if in.SetAutoOTA && in.AutoOTA != nil {
		pr.AutoOTA = *in.AutoOTA
	}
	autoOTA := 0
	if pr.AutoOTA {
		autoOTA = 1
	}
	_, err = p.DB.Exec(`UPDATE products SET name=?, description=?, topic_style_default=?, auto_ota=?, methods_json=?, metrics_json=?, attributes_json=?, updated_at=? WHERE id=?`,
		pr.Name, pr.Description, pr.TopicStyleDefault, autoOTA,
		mustJSON(pr.Methods), mustJSON(pr.Metrics), mustJSON(pr.Attributes),
		db.NowMS(), id)
	if err != nil {
		return nil, err
	}
	return p.GetProduct(id)
}

func (p *Platform) DeleteProduct(id int64) error {
	var deviceCount int
	_ = p.DB.QueryRow(`SELECT COUNT(1) FROM devices WHERE product_pk=?`, id).Scan(&deviceCount)
	if deviceCount > 0 {
		return fmt.Errorf("product has %d device(s); delete or move devices first", deviceCount)
	}
	res, err := p.DB.Exec(`DELETE FROM products WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("not found")
	}
	return nil
}

func (p *Platform) ApplyProductESPDefaults(id int64) (*models.Product, error) {
	pr, err := p.GetProduct(id)
	if err != nil {
		return nil, err
	}
	// Re-apply preset definitions; keep product-specific methods/metrics.
	presetMethods := PresetMethodNames()
	presetMetrics := PresetMetricKeys()
	customMethods := make([]models.ProductMethod, 0)
	for _, m := range pr.Methods {
		if m.Name == "" || presetMethods[m.Name] || IsBuiltinRPCMethod(m.Name) {
			continue
		}
		customMethods = append(customMethods, m)
	}
	customMetrics := make([]models.ProductMetric, 0)
	for _, m := range pr.Metrics {
		if m.Key != "" && !presetMetrics[m.Key] {
			customMetrics = append(customMetrics, m)
		}
	}
	pr.Methods = mergeMethods(DefaultESPMethods(), customMethods)
	pr.Metrics = mergeMetrics(DefaultESPMetrics(), customMetrics)
	_, err = p.DB.Exec(`UPDATE products SET methods_json=?, metrics_json=?, updated_at=? WHERE id=?`,
		mustJSON(pr.Methods), mustJSON(pr.Metrics), db.NowMS(), id)
	if err != nil {
		return nil, err
	}
	return p.GetProduct(id)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	if len(b) == 0 {
		return "[]"
	}
	return string(b)
}

func DefaultESPMethods() []models.ProductMethod {
	return []models.ProductMethod{
		{
			Name: "rpc-list", Label: "RPC list", Description: "Built-in firmware method catalog (alias rpc_list). Not user-registerable.",
		},
		{
			Name: "reboot", Label: "Reboot", Description: "Restart the device",
			Params: []models.ProductMethodParam{
				{Name: "delayMs", Label: "Delay (ms)", Type: "number", Default: float64(500)},
			},
		},
	}
}

func DefaultESPMetrics() []models.ProductMetric {
	return []models.ProductMetric{
		{Key: "cpuUsagePercent", Label: "CPU usage", Group: "System", Unit: "%", Format: "percent", Description: "CPU utilization"},
		{Key: "uptimeSeconds", Label: "Uptime", Group: "System", Unit: "s", Format: "duration", Description: "Time since boot"},
		{Key: "wifiConnected", Label: "Wi‑Fi", Group: "Network", Format: "bool", Description: "Wi‑Fi association state"},
		{Key: "wifiIp", Label: "IP address", Group: "Network", Format: "text", Description: "Station IP"},
		{Key: "wifiRssi", Label: "Wi‑Fi signal", Group: "Network", Unit: "dBm", Format: "rssi", Description: "Received signal strength"},
		{Key: "freeHeap", Label: "Free heap", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Free internal RAM"},
		{Key: "freeDram", Label: "Free DRAM", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Free internal DRAM"},
		{Key: "minFreeDram", Label: "Min free DRAM", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Lowest free DRAM watermark"},
		{Key: "largestFreeDramBlock", Label: "Largest free DRAM block", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Largest contiguous free DRAM block"},
		{Key: "totalDram", Label: "Total DRAM", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Total internal DRAM size"},
		{Key: "minFreeHeap", Label: "Min free heap", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Lowest free heap watermark"},
		{Key: "largestFreeBlock", Label: "Largest free block", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Largest contiguous free heap block"},
		{Key: "freePsram", Label: "Free PSRAM", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Free external PSRAM"},
		{Key: "minFreePsram", Label: "Min free PSRAM", Group: "Memory", Unit: "bytes", Format: "bytes", Description: "Lowest free PSRAM watermark"},
		{Key: "storageTotalBytes", Label: "Storage total", Group: "Storage", Unit: "bytes", Format: "bytes", Description: "Filesystem capacity"},
		{Key: "storageUsedBytes", Label: "Storage used", Group: "Storage", Unit: "bytes", Format: "bytes", Description: "Filesystem used space"},
	}
}

// IsBuiltinRPCMethod reports SDK-reserved names that firmware always provides
// and that must not be registered on the product schema.
func IsBuiltinRPCMethod(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rpc-list", "rpc_list":
		return true
	default:
		return false
	}
}

// MergeProductPresets ensures built-in reboot method and ESP metrics always exist.
// Product-defined entries with the same name/key win, except reserved rpc-list.
func MergeProductPresets(pr *models.Product) {
	if pr == nil {
		return
	}
	pr.Methods = mergeMethods(DefaultESPMethods(), pr.Methods)
	pr.Metrics = mergeMetrics(DefaultESPMetrics(), pr.Metrics)
	if pr.Attributes == nil {
		pr.Attributes = []models.ProductAttribute{}
	}
}

func mergeMethods(presets, custom []models.ProductMethod) []models.ProductMethod {
	byName := map[string]models.ProductMethod{}
	order := []string{}
	for _, m := range presets {
		byName[m.Name] = m
		order = append(order, m.Name)
	}
	for _, m := range custom {
		if m.Name == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(m.Name), "rpc_list") {
			continue // alias of built-in rpc-list; not a separate method
		}
		if _, ok := byName[m.Name]; !ok {
			order = append(order, m.Name)
		}
		byName[m.Name] = m
	}
	out := make([]models.ProductMethod, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out
}

func mergeMetrics(presets, custom []models.ProductMetric) []models.ProductMetric {
	byKey := map[string]models.ProductMetric{}
	order := []string{}
	for _, m := range presets {
		byKey[m.Key] = m
		order = append(order, m.Key)
	}
	for _, m := range custom {
		if m.Key == "" {
			continue
		}
		if _, ok := byKey[m.Key]; !ok {
			order = append(order, m.Key)
		}
		byKey[m.Key] = m
	}
	out := make([]models.ProductMetric, 0, len(order))
	for _, k := range order {
		out = append(out, byKey[k])
	}
	return out
}

func PresetMethodNames() map[string]bool {
	out := map[string]bool{}
	for _, m := range DefaultESPMethods() {
		out[m.Name] = true
	}
	out["rpc_list"] = true
	return out
}

func PresetMetricKeys() map[string]bool {
	out := map[string]bool{}
	for _, m := range DefaultESPMetrics() {
		out[m.Key] = true
	}
	return out
}
