package hyperliquid

type LimitOrderType struct {
	TimeInForce string
}

type TriggerOrderType struct {
	TriggerPrice float64
	IsMarket     bool
	TPSL         string
}

type OrderType struct {
	Limit   *LimitOrderType
	Trigger *TriggerOrderType
}

type OrderRequest struct {
	Coin          string
	IsBuy         bool
	Size          float64
	LimitPrice    float64
	OrderType     OrderType
	ReduceOnly    bool
	ClientOrderID string
}

type BuilderInfo struct {
	Address string
	Fee     int
}

type ModifyIdentifier struct {
	OrderID       *int64
	ClientOrderID string
}

type ModifyRequest struct {
	Identifier ModifyIdentifier
	Order      OrderRequest
}

type CancelRequest struct {
	Coin    string
	OrderID *int64
}

type CancelByCloidRequest struct {
	Coin          string
	ClientOrderID string
}

type SetReferrerRequest struct {
	Code string
}

type CreateSubAccountRequest struct {
	Name string
}

type USDClassTransferRequest struct {
	Amount float64
	ToPerp bool
}

type SendAssetRequest struct {
	Destination    string
	SourceDEX      string
	DestinationDEX string
	Token          string
	Amount         float64
}

type SubAccountTransferRequest struct {
	SubAccountUser string
	IsDeposit      bool
	USD            int64
}

type SubAccountSpotTransferRequest struct {
	SubAccountUser string
	IsDeposit      bool
	Token          string
	Amount         float64
}

type VaultUSDTransferRequest struct {
	VaultAddress string
	IsDeposit    bool
	USD          int64
}

type USDTransferRequest struct {
	Destination string
	Amount      float64
}

type SpotTransferRequest struct {
	Destination string
	Token       string
	Amount      float64
}

type TokenDelegateRequest struct {
	Validator    string
	Wei          uint64
	IsUndelegate bool
}

type WithdrawFromBridgeRequest struct {
	Destination string
	Amount      float64
}

type ApproveAgentRequest struct {
	AgentName string
}

type ApproveBuilderFeeRequest struct {
	Builder    string
	MaxFeeRate string
}

type ConvertToMultiSigUserRequest struct {
	AuthorizedUsers []string
	Threshold       int
}

type SpotDeployRegisterTokenRequest struct {
	TokenName    string
	SizeDecimals int
	WeiDecimals  int
	MaxGas       int
	FullName     string
}

type SpotDeployUserGenesisEntry struct {
	User string
	Wei  string
}

type SpotDeployExistingTokenWeiEntry struct {
	Token int
	Wei   string
}

type SpotDeployUserGenesisRequest struct {
	Token               int
	UserAndWei          []SpotDeployUserGenesisEntry
	ExistingTokenAndWei []SpotDeployExistingTokenWeiEntry
}

type SpotDeployFreezeUserRequest struct {
	Token  int
	User   string
	Freeze bool
}

type SpotDeployGenesisRequest struct {
	Token            int
	MaxSupply        string
	NoHyperliquidity bool
}

type SpotDeployRegisterSpotRequest struct {
	BaseToken  int
	QuoteToken int
}

type SpotDeployRegisterHyperliquidityRequest struct {
	Spot         int
	StartPrice   float64
	OrderSize    float64
	Orders       int
	SeededLevels *int
}

type SpotDeploySetDeployerTradingFeeShareRequest struct {
	Token int
	Share string
}

type PerpDeploySchema struct {
	FullName        string
	CollateralToken string
	OracleUpdater   *string
}

type PerpDeployRegisterAssetRequest struct {
	Dex           string
	MaxGas        *int
	Coin          string
	SizeDecimals  int
	OraclePrice   string
	MarginTableID int
	OnlyIsolated  bool
	Schema        *PerpDeploySchema
}

type PerpDeploySetOracleRequest struct {
	Dex                string
	OraclePrices       map[string]string
	MarkPrices         []map[string]string
	ExternalPerpPrices map[string]string
}

type CValidatorRegisterRequest struct {
	NodeIP              string
	Name                string
	Description         string
	DelegationsDisabled bool
	CommissionBPS       int
	Signer              string
	Unjailed            bool
	InitialWei          uint64
}

type CValidatorChangeProfileRequest struct {
	NodeIP             *string
	Name               *string
	Description        *string
	Unjailed           bool
	DisableDelegations *bool
	CommissionBPS      *int
	Signer             *string
}

type MultiSigSignature struct {
	R string
	S string
	V int
}

type MultiSigRequest struct {
	MultiSigUser string
	Action       map[string]any
	Signatures   []MultiSigSignature
	Nonce        uint64
	VaultAddress *string
}

type UseBigBlocksRequest struct {
	Enable bool
}

type UserDexAbstractionRequest struct {
	User    string
	Enabled bool
}

type NoopRequest struct {
	Nonce uint64
}
