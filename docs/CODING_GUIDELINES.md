# GoCryptoTrader Coding Guidelines

This document outlines the coding, formatting, and testing standards for implementing or refactoring exchange API integrations or any related functionality within the codebase. These practices ensure consistency, maintainability, and performance throughout the project.

## General Standards

- Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting) guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
- Code must adhere to these [Effective Go](https://go.dev/doc/effective_go) guidelines.
- Code must also follow these [Go Style](https://google.github.io/styleguide/go/) guidelines.

## Security

See [SECURITY.md](/SECURITY.md) for the project's security policy, supported versions and reporting process.

- Never commit API keys, secrets, client IDs or a populated `config.json`. Use placeholder values in tests and examples.
- Never log or return credentials in error messages, RPC responses or test output.
- If you discover a vulnerability while working on the codebase, report it privately as described in [SECURITY.md](/SECURITY.md). Do not describe it in a public issue, pull request or commit message.

## Exchange Implementation Guidelines

Refer to the [ADD_NEW_EXCHANGE.md](/docs/ADD_NEW_EXCHANGE.md) document for comprehensive steps on integrating a new exchange.

### Endpoint Organisation

- Implement API endpoints in the order they are presented in the API documentation to maintain alignment with the source.
- Group related endpoints into files that follow the documented API structure.
- Inline endpoint paths directly in the method implementation. Avoid defining them as constants elsewhere.
- Export exchange types, functions and methods by default (e.g. `func (e *Exchange) GetOrderBook(...)`) so that GoCryptoTrader can be consumed as both a standalone library and interfaced via the engine package.

### Type Usage

- Use the most appropriate native Go types for struct fields:
  - If the API always returns a number as a bare JSON number, use `float64`.
  - If the API returns a number as a quoted JSON string, or may return either a string or a bare number, use `types.Number` — **do not** use `float64` with the `json:",string"` tag.
  - For timestamps, use `time.Time` if Go's JSON unmarshalling supports the format directly; otherwise use `types.Time` for Unix timestamps that require custom unmarshalling.
- Always use full and descriptive field names for clarity and consistency. Avoid short API-provided aliases unless compatibility requires it.
- Default to `uint64` for exchange API parameters and structs for integers where appropriate.
  - Avoid `int` (size varies by architecture) or `int64` (allows negatives where they don't make sense).
  - Aligns well with `strconv.FormatUint`.

### TestMain usage

- TestMain must avoid API calls, so that individual unit tests can run quickly. Use sync.Once or similar patterns to bootstrap common data without burdening all unit tests with the same overhaed. See `UpdatePairsOnce` for an example of this.

### Struct Naming

- Request structs must be named in the form `XRequest`.
- Response structs must be named in the form `XResponse`.
- Request structs should be passed as pointers. Decode response structs into their zero value and return a pointer only after successful decoding:

```go
	var x XResponse
	if err := e.SendHTTPRequest(ctx, endpoint, path, &x); err != nil {
		return nil, err
	}
	return &x, nil
```

### Parameter Handling

- Use pointer structs for passing request parameters.
- Use idiomatic Go types (e.g., `time.Time`) in the parameter definition and convert them within the method as needed when preparing the request.
- Time related requests should default to UTC.

#### Caller-oriented request parameters

- Public request structs must model caller intent, not the exchange wire representation.
- Fields must use the strongest established native or domain type available, such as `currency.Pair`, `currency.Code`, `asset.Item`, `order.Side`, `order.Type`, `kline.Interval`, `time.Time`, `time.Duration`, booleans, and fixed-width numeric types.
- Absolute times must use `time.Time`. Durations and relative windows must use `time.Duration` or an existing semantic interval type. Endpoints must perform UTC normalisation, unit conversion, and formatting.
- Non-negative whole values should use `uint64`. Signed types are appropriate only when negative values have documented meaning.
- Fractional numeric inputs such as prices, quantities, volumes, fees, and amounts should use `float64`; they must not be strings merely because the exchange transmits quoted numbers. Endpoints must reject NaN, infinity, and invalid ranges and format values without scientific notation.
- `types.Number` must not be used merely as a caller-facing request number. It is intended for flexible response decoding.
- Numeric-looking IDs, cursors, tokens, transaction references, and account identifiers must remain strings when arithmetic is meaningless; their exact representation must be preserved.
- Enumerated values must use existing domain types or exchange-local named types and constants where the mapping is stable. Free-form strings are acceptable only for genuinely open protocol values.
- Repeated values must be slices or domain collections. CSV strings, JSON arrays, and other joined representations must be produced inside the endpoint.
- A pointer scalar must be used when omission differs from an explicit zero, empty value, or `false`. Otherwise, prefer the value type. Every field's zero-value and omission behaviour must be defined.
- A wire field accepting multiple concepts, such as absolute time, relative duration, or an opaque token, should be represented by separate typed fields with exactly-one validation or by a small dedicated validated type. Raw wire-expression strings are a documented escape hatch, not the default.
- Endpoint methods must validate before credential lookup or network activity, convert into local `url.Values` or an unexported wire payload, and must not mutate the caller's request.
- Every conversion and validation path must have direct tests asserting the exact outbound query or body, including omitted values, explicit zero or `false`, boundaries, UTC conversion, very small decimals, and polymorphic fields.

### Path Construction

- Path API endpoints must be inlined within the calling method.
- Use basic string concatenation instead of `fmt.Sprintf`:

```go
    path := "/api/v1/" + id
```

- For multi-part strings, consider using `strings.Builder`:
  - Use only after benchmarking with `testing.B` to ensure it improves performance for realistic input sizes.

- Use the following function:

```go
    path = common.EncodeURLValues(path, params)
```

  to append query parameters efficiently. This handles both empty and set params and will automatically handle the "?" for you.

## Error Handling

- Wrap external errors using fmt.Errorf with context, using the following format:

```go
    return nil, fmt.Errorf("error fetching order: %w", err)
```

- You may define and return your own custom errors when appropriate, especially for known API error codes or validation failures:

```go
    var errInvalidSymbol = errors.New("invalid symbol provided")

    if symbol == "" {
        return nil, errInvalidSymbol
    }
```

- Prefer package-level sentinel errors for validation, parsing, and custom unmarshalling failures that produce deterministic, testable outcomes when callers or tests need stable `errors.Is` matching.
- When returning additional context for a sentinel error, wrap the sentinel with `fmt.Errorf` and `%w` rather than replacing it with a new inline `errors.New(...)` value.

```go
    var errInvalidOrderSide = errors.New("invalid order side")

    if side == "" {
        return fmt.Errorf("%w: empty input", errInvalidOrderSide)
    }
```

- Prefer meaningful and specific error messages that identify the operation being performed.
- Always include enough context in errors to aid in debugging and traceability.
- Do not use panic; always return and propagate errors cleanly.

## Testing Guidelines

### General testing

Verify all tests pass by:

```console
    go test ./... -race -count 1
```

### Assertion Usage

Use `require` and `assert` appropriately:

#### require

- Use when test flow depends on the result.
- Messages must contain **"must"** (e.g., "response must not be nil").

#### assert

- Use when the test can proceed regardless of the check.
- Messages must contain **"should"** (e.g., "status code should be 200").

#### `f` variants (`assert.ErrorIsf`, `require.NoErrorf`, etc.)

- Only use `f` variants when the message contains **format verbs** (e.g., `%s`, `%d`, `%v`).
- If the message is a plain string with no format verbs, use the non-`f` variant.

```go
    // Correct — format verb %s requires the f variant:
    assert.NoErrorf(t, err, "UpdateAccountInfo should not error for asset %s", a)

    // Correct — plain message, no format verbs:
    assert.ErrorIs(t, err, errInvalidOrderSize, "validate should return expected error")

    // Wrong — f variant used without format verbs:
    assert.ErrorIsf(t, err, errInvalidOrderSize, "validate should return expected error")
```

#### Sentinel error coverage

- When returning sentinel errors directly or wrapped, add direct `assert.ErrorIs` or `require.ErrorIs` coverage for that path.
- For validation and custom unmarshalling helpers, prefer a focused test for the exact sentinel error path rather than relying only on indirect endpoint coverage.

### Test Coverage

- Maintain original test inputs unless they are incorrect.
- Full test coverage is preferable; mock external calls as needed.
- All unit tests must pass before finalising changes.

### Test deduplication

- Test deduplication should be the default approach for exchanges and across the codebase, an example can be seen below:

```diff
--- a/gateio_test.go
+++ b/gateio_test.go
@@ -89,19 +89,11 @@ func TestGetAccountInfo(t *testing.T) {
     t.Parallel()
     sharedtestvalues.SkipTestIfCredentialsUnset(t, g)
-    _, err := g.UpdateAccountInfo(t.Context(), asset.Spot)
-    if err != nil {
-        t.Error("GetAccountInfo() error", err)
-    }
-    if _, err := g.UpdateAccountInfo(t.Context(), asset.Margin); err != nil {
-        t.Errorf("%s UpdateAccountInfo() error %v", g.Name, err)
-    }
-    if _, err := g.UpdateAccountInfo(t.Context(), asset.CrossMargin); err != nil {
-        t.Error("%s UpdateAccountInfo() error %v", g.Name, err)
-    }
-    if _, err := g.UpdateAccountInfo(t.Context(), asset.Options); err != nil {
-        t.Error("%s UpdateAccountInfo() error %v", g.Name, err)
-    }
-    if _, err := g.UpdateAccountInfo(t.Context(), asset.Futures); err != nil {
-        t.Error("%s UpdateAccountInfo() error %v", g.Name, err)
-    }
-    if _, err := g.UpdateAccountInfo(t.Context(), asset.DeliveryFutures); err != nil {
-        t.Error("%s UpdateAccountInfo() error %v", g.Name, err)
-    }
+    for _, a := range g.GetAssetTypes(false) {
+        _, err := g.UpdateAccountInfo(t.Context(), a)
+        assert.NoErrorf(t, err, "UpdateAccountInfo should not error for asset %s", a)
+    }
}
```

## Comments

- API methods and public types must have comments for GoDoc.
- Comments should explain **why** the code is doing something, not **what** it's doing, which should be self-explanatory.
- Self-explanatory comments must be avoided.
- Only retain comments for complex logic or where external behaviour needs clarification.

## Formatting

Run the following after completing changes:

```console
    make gofumpt
```

This ensures proper formatting across the codebase.

## Linters and other miscellaneous checks

Run the following to check for linting issues:

```console
    golangci-lint run ./... (or make lint)
```

Run the miscellaneous repository checks locally with:

```console
    make misc_checks
```

The full local verification flow can be run with:

```console
    make check
```

This includes linting, miscellaneous checks and tests. The same miscellaneous checks are also run via [GitHub actions](/.github/workflows/misc.yml).

- All lint warnings and errors must be resolved before merging.
- Use `//nolint:linter-name` sparingly and always explain the reason in a comment next to the code.
- Examples of valid use:

```go
    extension := "strat" //nolint:misspell // its shorthand for strategy
```
