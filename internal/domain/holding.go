package domain

import "strconv"

// AssetType categorizes an investment holding.
type AssetType string

const (
	MutualFund AssetType = "Mutual Fund"
	StockAsset AssetType = "Stock"
	ETFAsset   AssetType = "ETF"
	GoldAsset  AssetType = "Gold"
	CryptoAsset AssetType = "Crypto"
	OtherAsset AssetType = "Other"
)

var ValidAssetTypes = []AssetType{MutualFund, StockAsset, ETFAsset, GoldAsset, CryptoAsset, OtherAsset}

func IsValidAssetType(t AssetType) bool {
	for _, v := range ValidAssetTypes {
		if v == t {
			return true
		}
	}
	return false
}

// UnitScale is the fixed-point scale for fractional units (micro-units): a
// Holding.UnitsMicro of 142_317_000 means 142.317 units.
const UnitScale = 1_000_000

// Holding is an investment position. Units are stored as micro-units for
// fractional precision; AvgCost and LastPrice are paise *per unit*.
type Holding struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        AssetType `json:"type"`
	UnitsMicro  int64     `json:"units_micro"` // units × 1e6
	AvgCost     Money     `json:"avg_cost"`    // paise per unit
	LastPrice   Money     `json:"last_price"`  // paise per unit (0 = unknown)
	SchemeCode  string    `json:"scheme_code"` // AMFI scheme code (MF) or ticker
	LastPriceAt string    `json:"last_price_at,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// Units returns the human-readable unit count.
func (h Holding) Units() float64 { return float64(h.UnitsMicro) / UnitScale }

// UnitsToMicro converts a decimal unit count to micro-units.
func UnitsToMicro(units float64) int64 {
	return int64(units*UnitScale + sign(units)*0.5)
}

// FormatUnits renders micro-units with up to 3 decimals, trimming zeros.
func FormatUnits(micro int64) string {
	s := strconv.FormatFloat(float64(micro)/UnitScale, 'f', 3, 64)
	// trim trailing zeros and a dangling dot
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}
