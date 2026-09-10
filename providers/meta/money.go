package meta

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// Manifests declare Meta money in account-currency units, so `20` in a USD
// account means USD 20.00. The Graph API instead expects the account
// currency's minimum denomination, and how many of those fit in one
// account-currency unit is Meta's per-currency offset. Amounts are therefore
// parsed into a currency-independent fixed-point form and converted to the API
// unit only once the ad account currency is known.
const (
	// amountScale is the fixed-point scale of an amount. Every Meta currency
	// offset is 1 or 100, so two decimal places is the finest precision any
	// Meta ad account currency can express.
	amountScale = int64(100)
	// amountDecimals is amountScale expressed as decimal places.
	amountDecimals = 2
	// floatAmountTolerance absorbs binary floating-point error when a manifest
	// value such as 20.55 is scaled to fixed point.
	floatAmountTolerance = 1e-6
)

// currencyOffsets is Meta's published offset per ad account currency: the
// number of minimum-denomination units in one account-currency unit. Meta's
// offsets intentionally differ from ISO 4217 for currencies such as HUF, ISK,
// IDR, and TWD, and Meta charges Bahraini, Jordanian, and Kuwaiti dinars in
// hundredths rather than thousandths.
//
// Source: https://developers.facebook.com/docs/marketing-api/currencies/
var currencyOffsets = map[string]int64{
	"AED": 100, "ARS": 100, "AUD": 100, "BDT": 100, "BGN": 100, "BHD": 100,
	"BOB": 100, "BRL": 100, "CAD": 100, "CHF": 100, "CLP": 1, "CNY": 100,
	"COP": 1, "CRC": 1, "CZK": 100, "DKK": 100, "DZD": 100, "EGP": 100,
	"EUR": 100, "FBZ": 100, "GBP": 100, "GTQ": 100, "HKD": 100, "HNL": 100,
	"HRK": 100, "HUF": 1, "IDR": 1, "ILS": 100, "INR": 100, "ISK": 1,
	"JOD": 100, "JPY": 1, "KES": 100, "KRW": 1, "LTL": 100, "LVL": 100,
	"MOP": 100, "MXN": 100, "MYR": 100, "NGN": 100, "NIO": 100, "NOK": 100,
	"NZD": 100, "PEN": 100, "PHP": 100, "PKR": 100, "PLN": 100, "PYG": 1,
	"QAR": 100, "RON": 100, "RSD": 100, "RUB": 100, "SAR": 100, "SEK": 100,
	"SGD": 100, "SKK": 100, "THB": 100, "TRY": 100, "TWD": 1, "UAH": 100,
	"USD": 100, "UYU": 100, "VEF": 100, "VES": 100, "VND": 1, "ZAR": 100,
}

// amount is a positive money value in account-currency units scaled by
// amountScale. It is deliberately not the API's minimum denomination so that
// manifests can be validated and compared without knowing the ad account
// currency.
type amount int64

// currencyResolver reports the ad account currency. Resources that declare no
// money never call it, so a budget-free manifest needs no currency read.
type currencyResolver func() (string, error)

// currencyOffset returns how many minimum-denomination units make up one unit
// of currency.
func currencyOffset(currency string) (int64, error) {
	offset, ok := currencyOffsets[currency]
	if !ok {
		return 0, fmt.Errorf("ad account currency %q is not a supported Meta advertising currency", currency)
	}
	return offset, nil
}

// parseAmount converts a manifest value in account-currency units into fixed
// point. Errors are phrased for manifest authors because they surface during
// validation.
func parseAmount(v any) (amount, error) {
	switch x := v.(type) {
	case string:
		return parseDecimalAmount(strings.TrimSpace(x))
	case json.Number:
		return parseDecimalAmount(strings.TrimSpace(x.String()))
	case int:
		return wholeAmount(int64(x))
	case int32:
		return wholeAmount(int64(x))
	case int64:
		return wholeAmount(x)
	case float32:
		return floatAmount(float64(x))
	case float64:
		return floatAmount(x)
	default:
		return 0, fmt.Errorf("must be a positive amount in account-currency units, such as 20 or 20.50")
	}
}

func wholeAmount(n int64) (amount, error) {
	if n <= 0 {
		return 0, fmt.Errorf("must be greater than 0")
	}
	if n > math.MaxInt64/amountScale {
		return 0, fmt.Errorf("is too large")
	}
	return amount(n * amountScale), nil
}

func floatAmount(n float64) (amount, error) {
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0, fmt.Errorf("must be greater than 0")
	}
	scaled := n * float64(amountScale)
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) > floatAmountTolerance {
		return 0, fmt.Errorf("must have at most %d decimal places", amountDecimals)
	}
	if rounded <= 0 || rounded > float64(math.MaxInt64) {
		return 0, fmt.Errorf("is too large")
	}
	return amount(rounded), nil
}

func parseDecimalAmount(s string) (amount, error) {
	invalid := fmt.Errorf("must be a positive amount in account-currency units, such as 20 or 20.50")
	s = strings.TrimPrefix(s, "+")
	if s == "" || strings.ContainsAny(s, "eE") {
		return 0, invalid
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if hasFrac {
		if len(frac) > amountDecimals {
			return 0, fmt.Errorf("must have at most %d decimal places", amountDecimals)
		}
		frac += strings.Repeat("0", amountDecimals-len(frac))
	} else {
		frac = strings.Repeat("0", amountDecimals)
	}
	if !digitsOnly(whole) || !digitsOnly(frac) {
		return 0, invalid
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("is too large")
	}
	fraction, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, invalid
	}
	if units > math.MaxInt64/amountScale {
		return 0, fmt.Errorf("is too large")
	}
	scaled := units*amountScale + fraction
	if scaled <= 0 {
		return 0, fmt.Errorf("must be greater than 0")
	}
	return amount(scaled), nil
}

// value renders the amount the way a manifest declares it, so whole amounts
// stay integers in plan output and state.
func (a amount) value() any {
	if int64(a)%amountScale == 0 {
		return int64(a) / amountScale
	}
	return float64(a) / float64(amountScale)
}

// minimumDenomination converts the amount to the Graph API unit for currency.
func (a amount) minimumDenomination(currency string) (int64, error) {
	offset, err := currencyOffset(currency)
	if err != nil {
		return 0, err
	}
	perUnit := amountScale / offset
	if int64(a)%perUnit != 0 {
		return 0, fmt.Errorf("%v is not payable in %s; %s amounts must be whole numbers", a.value(), currency, currency)
	}
	return int64(a) / perUnit, nil
}

// setAmount writes an account-currency amount to a Graph API form field in the
// minimum denomination that field expects.
func setAmount(form url.Values, field, attr string, a amount, resolve currencyResolver) error {
	currency, err := resolve()
	if err != nil {
		return err
	}
	minimum, err := a.minimumDenomination(currency)
	if err != nil {
		return fmt.Errorf("attribute %q: %w", attr, err)
	}
	form.Set(field, strconv.FormatInt(minimum, 10))
	return nil
}

// amountFromMinimumDenomination converts a Graph API value back into an
// account-currency amount.
func amountFromMinimumDenomination(n int64, currency string) (amount, error) {
	offset, err := currencyOffset(currency)
	if err != nil {
		return 0, err
	}
	perUnit := amountScale / offset
	if n > math.MaxInt64/perUnit {
		return 0, fmt.Errorf("is too large")
	}
	return amount(n * perUnit), nil
}
