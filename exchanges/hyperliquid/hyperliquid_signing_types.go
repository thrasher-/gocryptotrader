package hyperliquid

type l1ActionRequest struct {
	Action       any
	VaultAddress string
	Nonce        uint64
	ExpiresAfter *uint64
}

type l1Signature struct {
	R string `json:"r"`
	S string `json:"s"`
	V uint8  `json:"v"`
}

type eip712Field struct {
	Name  string
	Type  string
	Value any
}

type signedActionRequest struct {
	Action       any         `json:"action"`
	Nonce        uint64      `json:"nonce"`
	Signature    l1Signature `json:"signature"`
	VaultAddress string      `json:"vaultAddress,omitempty"`
	ExpiresAfter *uint64     `json:"expiresAfter,omitempty"`
}
