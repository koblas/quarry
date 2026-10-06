package config

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// Keys of an [[acb.adjustment]] item.
const (
	securityKey               = "security"
	dateKey                   = "date"
	returnOfCapitalKey        = "return-of-capital"
	reinvestedDistributionKey = "reinvested-distribution"
)

// adjustmentSetting is the [[acb.adjustment]] list; its example is what a user whose acb is not a table is shown.
var adjustmentSetting = setting{table: "acb", name: "adjustment", example: "[[acb.adjustment]]"}

// adjustments is the [[acb.adjustment]] items in file order, nil when none. It refuses the first
// bad item, which holds the keys it needs before it holds them right.
func (d document) adjustments() ([]Adjustment, error) {
	value, present, err := d.lookup(adjustmentSetting)
	if err != nil || !present {
		return nil, err
	}
	items, isList := value.([]any)
	if _, inline := d.written(adjustmentSetting.key()); !isList || inline {
		return nil, d.badValue(adjustmentSetting.String()+" must be a list of tables, each under its own [[acb.adjustment]] line", d.got(adjustmentSetting.key()))
	}
	written := d.itemTexts()
	adjustments := make([]Adjustment, 0, len(items))
	for i, item := range items {
		// Not inline, so every item is a table and has the [[acb.adjustment]] header written[i] came from.
		fields, _ := item.(map[string]any)
		adjustment, err := d.adjustment(i+1, fields, written[i])
		if err != nil {
			return nil, err
		}
		adjustments = append(adjustments, adjustment)
	}

	return adjustments, nil
}

// itemTexts is, for each [[acb.adjustment]] header in file order, the text of each key written
// directly under it, as the file wrote it.
func (d document) itemTexts() []map[string]string {
	var texts []map[string]string
	for _, e := range d.entries {
		switch {
		case e.kind == unstable.ArrayTable && slices.Equal(e.key, adjustmentSetting.key()):
			texts = append(texts, map[string]string{})
		case e.kind == unstable.KeyValue && len(e.key) == 3 && slices.Equal(e.key[:2], adjustmentSetting.key()) && len(texts) > 0:
			texts[len(texts)-1][e.key[2]] = strings.Join(strings.Fields(e.value), " ")
		}
	}

	return texts
}

// adjustment is item n, whose keys are fields and whose text as written is texts. Missing keys
// are refused before wrong ones, in the order security, date, amount.
func (d document) adjustment(n int, fields map[string]any, texts map[string]string) (Adjustment, error) {
	name := adjustmentSetting.String() + " item " + strconv.Itoa(n)
	security, hasSecurity := fields[securityKey]
	date, hasDate := fields[dateKey]
	_, hasReturn := fields[returnOfCapitalKey]
	_, hasReinvested := fields[reinvestedDistributionKey]
	switch {
	case !hasSecurity:
		return Adjustment{}, d.badItem(name + ` needs security, such as security = "sec-41"`)
	case !hasDate:
		return Adjustment{}, d.badItem(name + " needs date, such as date = 2024-12-31")
	case !hasReturn && !hasReinvested:
		return Adjustment{}, d.badItem(name + " needs return-of-capital or reinvested-distribution, such as return-of-capital = 12.34")
	}

	id, isString := security.(string)
	if !isString || id == "" {
		return Adjustment{}, d.badValue(name+`: security must be a security id in quotes, such as "sec-41"`, textOf(texts, securityKey))
	}
	day, isDate := date.(toml.LocalDate)
	if !isDate {
		return Adjustment{}, d.badValue(name+": date must be a date such as 2024-12-31", textOf(texts, dateKey))
	}
	returned, err := d.amount(name, returnOfCapitalKey, fields, texts)
	if err != nil {
		return Adjustment{}, err
	}
	reinvested, err := d.amount(name, reinvestedDistributionKey, fields, texts)
	if err != nil {
		return Adjustment{}, err
	}

	return Adjustment{Security: id, Date: day.AsTime(time.UTC), ReturnOfCapital: returned, ReinvestedDistribution: reinvested}, nil
}

// amount is the cents of key as the item wrote it, 0 when it did not: text that money.ParseCents
// reads, above zero.
func (d document) amount(name, key string, fields map[string]any, texts map[string]string) (int64, error) {
	if _, given := fields[key]; !given {
		return 0, nil
	}
	text := textOf(texts, key)
	if cents, ok := money.ParseCents(text); ok && cents > 0 {
		return cents, nil
	}

	return 0, d.badValue(name+": "+key+" must be an amount in CAD above 0 with at most two decimals, such as 12.34", text)
}

// textOf is the text of key as the file wrote it, or "a table" when it was not written as a plain key.
func textOf(texts map[string]string, key string) string {
	if text, ok := texts[key]; ok {
		return text
	}

	return gotTable
}
