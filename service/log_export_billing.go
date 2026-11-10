package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

// Monetary columns use only the historical log snapshot, never live pricing or
// exchange rates. Unknown values stay empty, including in aggregated exports.
type logExportPriceKey struct{ expression, tier, group, rules string }

func logExportBillingValues(log model.Log, priceCache map[logExportPriceKey]map[string]decimal.Decimal) map[string]string {
	values := map[string]string{}
	if !gjson.Valid(log.Other) {
		return values
	}
	root := gjson.Parse(log.Other)
	for _, field := range []string{"billing_source", "billing_unit", "billing_quota_per_unit", "subscription_consumed"} {
		values[field] = root.Get(field).String()
	}
	tokens := root.Get("billing_charge_tokens")
	if !tokens.IsObject() {
		tokens = root.Get("billing_tokens")
	}
	if tokens.IsObject() {
		values["billing_tokens"] = tokens.Raw
	}
	prices := map[string]decimal.Decimal{}
	for key, value := range root.Get("billing_prices").Map() {
		if n, ok := logExportNumber(value); ok {
			prices[key] = n
		}
	}
	if root.Get("billing_mode").String() == "tiered_expr" {
		if data, err := base64.StdEncoding.DecodeString(root.Get("expr_b64").String()); err == nil {
			values["billing_expression"] = string(data)
			key := logExportPriceKey{string(data), root.Get("matched_tier").Raw, root.Get("billing_group_ratio").Raw, root.Get("request_rules").Raw}
			var cached bool
			prices, cached = priceCache[key]
			if !cached {
				prices = logExportExpressionPrices(string(data), root)
				if len(priceCache) < 256 {
					priceCache[key] = prices
				}
			}
		} else {
			prices = nil
		}
	}
	if len(prices) > 0 {
		encoded := map[string]json.Number{}
		for key, value := range prices {
			encoded[key] = json.Number(value.String())
		}
		if data, err := common.Marshal(encoded); err == nil {
			values["billing_prices"] = string(data)
		}
	}
	for field, path := range map[string]string{
		"input_tokens_total": "input_tokens_total", "cache_read_tokens": "cache_tokens", "cache_write_tokens": "cache_write_tokens",
		"cache_write_tokens_5m": "cache_creation_tokens_5m", "cache_write_tokens_1h": "cache_creation_tokens_1h",
		"image_input_tokens": "image_output", "image_output_tokens": "billing_tokens.img_o",
		"audio_input_tokens": "audio_input", "audio_output_tokens": "audio_output",
	} {
		if n, ok := logExportNumber(root.Get(path)); ok {
			values[field] = n.String()
		}
	}
	// Prefer supplier counts; normalized billing quantities are a fallback only
	// when the corresponding historical supplier field was not recorded.
	for field, key := range map[string]string{"input_tokens_total": "len", "cache_read_tokens": "cr", "image_input_tokens": "img", "image_output_tokens": "img_o", "audio_input_tokens": "ai", "audio_output_tokens": "ao"} {
		if values[field] == "" {
			if n, ok := logExportNumber(tokens.Get(key)); ok {
				values[field] = n.String()
			}
		}
	}

	if values["cache_write_tokens"] == "" {
		if n, ok := logExportNumber(root.Get("cache_creation_tokens")); ok {
			values["cache_write_tokens"] = n.String()
		} else {
			short, hasShort := logExportNumber(root.Get("cache_creation_tokens_5m"))
			long, hasLong := logExportNumber(root.Get("cache_creation_tokens_1h"))
			if hasShort || hasLong {
				values["cache_write_tokens"] = short.Add(long).String()
			}
		}
	}
	if values["cache_write_tokens"] == "" {
		total := decimal.Zero
		reported := false
		for _, key := range []string{"cc", "cc5m", "cc1h"} {
			if n, ok := logExportNumber(tokens.Get(key)); ok {
				total = total.Add(n)
				reported = true
			}
		}
		if reported {
			values["cache_write_tokens"] = total.String()
		}
	}
	if values["audio_input_tokens"] == "" {
		if n, ok := logExportNumber(root.Get("audio_input_token_count")); ok {
			values["audio_input_tokens"] = n.String()
		}
	}
	for field, key := range map[string]string{"billing_input_tokens": "p", "billing_output_tokens": "c"} {
		if n, ok := logExportNumber(tokens.Get(key)); ok {
			values[field] = n.String()
		}
	}
	units, hasUnits := logExportNumber(root.Get("billing_quota_per_unit"))
	quota := decimal.NewFromInt(int64(log.Quota))
	if root.Get("billing_source").String() == "subscription" {
		if consumed, ok := logExportNumber(root.Get("subscription_consumed")); ok {
			quota = consumed
		}
	}
	if hasUnits && units.IsPositive() {
		values["amount_usd"] = quota.Div(units).String()
	}
	// Each component must have both a saved quantity and a known effective price.
	// A missing price is not zero. Nonlinear/custom expressions retain their
	// original expression but do not invent a per-token decomposition.
	subtotal := decimal.Zero
	known := tokens.IsObject() && len(prices) > 0
	for key, quantity := range tokens.Map() {
		if key == "len" {
			continue
		}
		n, ok := logExportNumber(quantity)
		if !ok {
			known = false
			continue
		}
		price, priced := prices[key]
		if !priced && !n.IsZero() {
			known = false
		}
		if priced {
			subtotal = subtotal.Add(n.Mul(price).Div(decimal.NewFromInt(1000000)))
		}
	}
	for field, key := range map[string]string{"input_amount_usd": "p", "output_amount_usd": "c", "cache_read_amount_usd": "cr", "image_input_amount_usd": "img", "image_output_amount_usd": "img_o", "audio_input_amount_usd": "ai", "audio_output_amount_usd": "ao"} {
		n, ok := logExportNumber(tokens.Get(key))
		price, priced := prices[key]
		if ok && priced {
			values[field] = n.Mul(price).Div(decimal.NewFromInt(1000000)).String()
		}
	}
	write := decimal.Zero
	writeKnown := tokens.IsObject() && len(prices) > 0
	for _, key := range []string{"cc", "cc5m", "cc1h"} {
		quantity := tokens.Get(key)
		if !quantity.Exists() {
			continue
		}
		n, ok := logExportNumber(quantity)
		price, priced := prices[key]
		if !ok || (!priced && !n.IsZero()) {
			writeKnown = false
			continue
		}
		write = write.Add(n.Mul(price).Div(decimal.NewFromInt(1000000)))
	}
	if writeKnown {
		values["cache_write_amount_usd"] = write.String()
	}
	if known && values["amount_usd"] != "" {
		total, _ := decimal.NewFromString(values["amount_usd"])
		values["other_amount_usd"] = total.Sub(subtotal).String()
	}
	return values
}

func logExportNumber(value gjson.Result) (decimal.Decimal, bool) {
	if value.Type != gjson.Number {
		return decimal.Zero, false
	}
	n, err := decimal.NewFromString(value.Raw)
	return n, err == nil && !n.IsNegative()
}

// Decode only linear token expressions using the saved tier and rule traces.
// We never execute historical expressions against today's clock or requests.
func logExportExpressionPrices(expression string, root gjson.Result) map[string]decimal.Decimal {
	_, body := billingexpr.ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return nil
	}
	prices, ok := logExportPricePolynomial(tree.Node, root)
	group, groupOK := logExportNumber(root.Get("billing_group_ratio"))
	if !ok || !groupOK {
		return nil
	}
	if constant := prices[""]; !constant.IsZero() {
		return nil
	}
	delete(prices, "")
	for _, key := range []string{"p", "c", "cr", "cc", "cc1h", "img", "img_cr", "img_o", "ai", "ao"} {
		if _, exists := prices[key]; !exists {
			prices[key] = decimal.Zero
		}
	}
	for key, value := range prices {
		if value.IsNegative() {
			return nil
		}
		prices[key] = value.Mul(group)
	}
	return prices
}

type logExportTierMatch struct {
	name  string
	found bool
}

func (m *logExportTierMatch) Visit(node *ast.Node) {
	call, ok := (*node).(*ast.CallNode)
	if !ok || len(call.Arguments) != 2 {
		return
	}
	callee, ok := call.Callee.(*ast.IdentifierNode)
	if !ok || callee.Value != "tier" {
		return
	}
	name, ok := call.Arguments[0].(*ast.StringNode)
	if ok && name.Value == m.name {
		m.found = true
	}
}

func logExportPricePolynomial(node ast.Node, root gjson.Result) (map[string]decimal.Decimal, bool) {
	switch n := node.(type) {
	case *ast.IntegerNode:
		return map[string]decimal.Decimal{"": decimal.NewFromInt(int64(n.Value))}, true
	case *ast.FloatNode:
		return map[string]decimal.Decimal{"": decimal.NewFromFloat(n.Value)}, true
	case *ast.IdentifierNode:
		if strings.Contains("|p|c|cr|cc|cc1h|img|img_cr|img_o|ai|ao|", "|"+n.Value+"|") {
			return map[string]decimal.Decimal{n.Value: decimal.NewFromInt(1)}, true
		}
	case *ast.CallNode:
		callee, ok := n.Callee.(*ast.IdentifierNode)
		if !ok || callee.Value != "tier" || len(n.Arguments) != 2 {
			return nil, false
		}
		name, ok := n.Arguments[0].(*ast.StringNode)
		if ok && name.Value == root.Get("matched_tier").String() {
			return logExportPricePolynomial(n.Arguments[1], root)
		}
	case *ast.ConditionalNode:
		for _, rule := range root.Get("request_rules").Array() {
			if rule.Get("cond").String() == n.Cond.String() {
				if rule.Get("matched").Bool() {
					return logExportPricePolynomial(n.Exp1, root)
				}
				return logExportPricePolynomial(n.Exp2, root)
			}
		}
		// An untraced condition may select a historical tier, but must not
		// guess the branch of a clock/request-dependent multiplier.
		tier := root.Get("matched_tier").String()
		if tier == "" {
			return nil, false
		}
		left := &logExportTierMatch{name: tier}
		right := &logExportTierMatch{name: tier}
		branch := n.Exp1
		ast.Walk(&branch, left)
		branch = n.Exp2
		ast.Walk(&branch, right)
		if left.found != right.found {
			if left.found {
				return logExportPricePolynomial(n.Exp1, root)
			}
			return logExportPricePolynomial(n.Exp2, root)
		}
	case *ast.BinaryNode:
		left, lok := logExportPricePolynomial(n.Left, root)
		right, rok := logExportPricePolynomial(n.Right, root)
		if !lok || !rok {
			return nil, false
		}
		switch n.Operator {
		case "+", "-":
			for key, value := range right {
				if n.Operator == "-" {
					value = value.Neg()
				}
				left[key] = left[key].Add(value)
			}
			return left, true
		case "*":
			if _, scalar := left[""]; len(left) == 1 && scalar {
				left, right = right, left
			}
			scale, scalar := right[""]
			if len(right) != 1 || !scalar {
				return nil, false
			}
			for key, value := range left {
				left[key] = value.Mul(scale)
			}
			return left, true
		case "/":
			scale, scalar := right[""]
			if len(right) != 1 || !scalar || scale.IsZero() {
				return nil, false
			}
			for key, value := range left {
				left[key] = value.Div(scale)
			}
			return left, true
		}
	}
	return nil, false
}
