package hyperliquid

import (
	"fmt"

	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// MetaAndAssetContextsResponse holds meta data with associated asset contexts.
type MetaAndAssetContextsResponse struct {
	Meta          MetaResponse            `json:"meta"`
	AssetContexts []PerpetualAssetContext `json:"assetContexts"`
}

// UserStateResponse represents perpetual clearinghouse state for a user.
type UserStateResponse struct {
	Withdrawable  types.Number `json:"withdrawable"`
	MarginSummary struct {
		AccountValue    types.Number `json:"accountValue"`
		TotalMarginUsed types.Number `json:"totalMarginUsed"`
	} `json:"marginSummary"`
	AssetPositions []struct {
		Position struct {
			Coin       string       `json:"coin"`
			Szi        types.Number `json:"szi"`
			MarginUsed types.Number `json:"marginUsed"`
			Leverage   struct {
				Type   string       `json:"type"`
				Value  types.Number `json:"value"`
				RawUSD types.Number `json:"rawUsd"`
			} `json:"leverage"`
		} `json:"position"`
	} `json:"assetPositions"`
}

// SpotUserStateResponse captures spot clearinghouse balances for a user.
type SpotUserStateResponse struct {
	Balances []struct {
		Coin   string       `json:"coin"`
		Token  int64        `json:"token"`
		Total  types.Number `json:"total"`
		Hold   types.Number `json:"hold"`
		Entry  types.Number `json:"entryNtl"`
		Symbol string       `json:"symbol"`
	} `json:"balances"`
}

// UnmarshalJSON allows the metaAndAssetCtxs array payload to be decoded into a structured response.
func (r *MetaAndAssetContextsResponse) UnmarshalJSON(data []byte) error {
	var payload []json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("hyperliquid: decode meta contexts: %w", err)
	}
	if len(payload) < 2 {
		return errMetaAndAssetContextsMalformed
	}
	if err := json.Unmarshal(payload[0], &r.Meta); err != nil {
		return fmt.Errorf("hyperliquid: decode meta: %w", err)
	}
	if err := json.Unmarshal(payload[1], &r.AssetContexts); err != nil {
		return fmt.Errorf("hyperliquid: decode asset contexts: %w", err)
	}
	return nil
}

// SpotAssetContext represents the spot asset context payload.
type SpotAssetContext struct {
	DayNotionalVolume  types.Number  `json:"dayNtlVlm"`
	MarkPrice          types.Number  `json:"markPx"`
	MidPrice           *types.Number `json:"midPx"`
	PrevDayPrice       types.Number  `json:"prevDayPx"`
	CirculatingSupply  types.Number  `json:"circulatingSupply"`
	Coin               string        `json:"coin"`
	AvailableLiquidity *types.Number `json:"availableLiquidity,omitempty"`
}

// SpotMetaAndAssetContextsResponse holds spot meta data and contexts.
type SpotMetaAndAssetContextsResponse struct {
	Meta          SpotMetaResponse   `json:"meta"`
	AssetContexts []SpotAssetContext `json:"assetContexts"`
}

// UserFundingHistoryEntry represents a single delta entry from user funding history responses.
type UserFundingHistoryEntry struct {
	Delta struct {
		Coin        string       `json:"coin"`
		FundingRate string       `json:"fundingRate"`
		NSamples    int64        `json:"nSamples"`
		Szi         types.Number `json:"szi"`
		Type        string       `json:"type"`
		USDC        types.Number `json:"usdc"`
	} `json:"delta"`
	Hash string     `json:"hash"`
	Time types.Time `json:"time"`
}

// UserNonFundingLedgerEntry captures user ledger updates unrelated to funding.
type UserNonFundingLedgerEntry struct {
	Time  types.Time `json:"time"`
	Hash  string     `json:"hash"`
	Delta struct {
		Type             string       `json:"type"`
		USDC             types.Number `json:"usdc"`
		Fee              types.Number `json:"fee"`
		Destination      string       `json:"destination"`
		User             string       `json:"user"`
		Vault            string       `json:"vault"`
		RequestedUSD     types.Number `json:"requestedUsd"`
		NetWithdrawnUSD  types.Number `json:"netWithdrawnUsd"`
		Commission       types.Number `json:"commission"`
		ClosingCost      types.Number `json:"closingCost"`
		CloseSize        types.Number `json:"closeSz"`
		ClosingDirection string       `json:"closingDir"`
		OpenSize         types.Number `json:"openSz"`
		OpenDirection    string       `json:"openDir"`
		Referrer         string       `json:"referrer"`
		Reward           types.Number `json:"reward"`
		Token            *string      `json:"token,omitempty"`
		Nonce            int64        `json:"nonce"`
	} `json:"delta"`
}

// UserFeesResponse models the fee history response for a user.
type UserFeesResponse struct {
	UserAddRate   types.Number `json:"userAddRate"`
	UserCrossRate types.Number `json:"userCrossRate"`
	FeeSchedule   struct {
		Add   types.Number `json:"add"`
		Cross types.Number `json:"cross"`
	} `json:"feeSchedule"`
}

// FundingHistoryEntry represents a single funding history point.
type FundingHistoryEntry struct {
	Coin     string       `json:"coin"`
	Funding  types.Number `json:"fundingRate"`
	Time     types.Time   `json:"time"`
	Period   string       `json:"period"`
	Premium  types.Number `json:"premium"`
	Bound    string       `json:"bound"`
	Interval string       `json:"interval"`
	OraclePx types.Number `json:"oraclePx"`
	MarkPx   types.Number `json:"markPx"`
	NextFR   types.Number `json:"nextFundingRate"`
}

// OpenOrderResponse mirrors the structure provided by the info open order endpoints.
type OpenOrderResponse struct {
	Coin        string       `json:"coin"`
	LimitPrice  types.Number `json:"limitPx"`
	OrderID     int64        `json:"oid"`
	Side        string       `json:"side"`
	Size        types.Number `json:"sz"`
	Timestamp   types.Time   `json:"timestamp"`
	ReduceOnly  bool         `json:"reduceOnly"`
	OrderType   string       `json:"orderType"`
	TimeInForce string       `json:"tif"`
	ClientOID   *string      `json:"cloid"`
}

// HistoricalOrderEntry provides order status history entries.
type HistoricalOrderEntry struct {
	Order struct {
		Coin        string       `json:"coin"`
		Side        string       `json:"side"`
		LimitPrice  types.Number `json:"limitPx"`
		Size        types.Number `json:"sz"`
		OrigSize    types.Number `json:"origSz"`
		OrderID     int64        `json:"oid"`
		Timestamp   types.Time   `json:"timestamp"`
		TriggerPx   types.Number `json:"triggerPx"`
		OrderType   string       `json:"orderType"`
		TimeInForce string       `json:"tif"`
		ReduceOnly  bool         `json:"reduceOnly"`
		ClientOID   *string      `json:"cloid"`
	} `json:"order"`
	Status          string     `json:"status"`
	StatusTimestamp types.Time `json:"statusTimestamp"`
}

// UnmarshalJSON allows the spotMetaAndAssetCtxs array payload to be decoded into a structured response.
func (r *SpotMetaAndAssetContextsResponse) UnmarshalJSON(data []byte) error {
	var payload []json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("hyperliquid: decode spot meta contexts: %w", err)
	}
	if len(payload) < 2 {
		return errSpotMetaAndAssetContextsMalformed
	}
	if err := json.Unmarshal(payload[0], &r.Meta); err != nil {
		return fmt.Errorf("hyperliquid: decode spot meta: %w", err)
	}
	if err := json.Unmarshal(payload[1], &r.AssetContexts); err != nil {
		return fmt.Errorf("hyperliquid: decode spot asset contexts: %w", err)
	}
	return nil
}

// UserFill represents a single user fill entry.
type UserFill struct {
	Coin           string       `json:"coin"`
	Price          types.Number `json:"px"`
	Size           types.Number `json:"sz"`
	Side           string       `json:"side"`
	Time           types.Time   `json:"time"`
	StartPosition  types.Number `json:"startPosition"`
	Direction      string       `json:"dir"`
	ClosedPnl      types.Number `json:"closedPnl"`
	Hash           string       `json:"hash"`
	OrderID        int64        `json:"oid"`
	Crossed        bool         `json:"crossed"`
	Fee            types.Number `json:"fee"`
	FeeToken       *string      `json:"feeToken,omitempty"`
	TradeID        *int64       `json:"tid,omitempty"`
	SliceID        *int64       `json:"sliceId,omitempty"`
	ParentOrderID  *int64       `json:"parentOid,omitempty"`
	ExecutionVenue *string      `json:"venue,omitempty"`
}

// PerpDexStreamingOiCap models a single asset to streaming open interest cap entry.
type PerpDexStreamingOiCap struct {
	Asset string
	Cap   types.Number
}

// UnmarshalJSON decodes tuple encoded open interest cap entries.
func (c *PerpDexStreamingOiCap) UnmarshalJSON(data []byte) error {
	var payload []json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("hyperliquid: decode perp dex streaming oi cap: %w", err)
	}
	if len(payload) != 2 {
		return errStreamingOiCapMalformed
	}
	if err := json.Unmarshal(payload[0], &c.Asset); err != nil {
		return fmt.Errorf("hyperliquid: decode streaming oi asset: %w", err)
	}
	if err := json.Unmarshal(payload[1], &c.Cap); err != nil {
		return fmt.Errorf("hyperliquid: decode streaming oi cap: %w", err)
	}
	return nil
}

// PerpDex represents a builder deployed perpetual dex configuration.
type PerpDex struct {
	Name                  string                  `json:"name"`
	FullName              string                  `json:"fullName"`
	Deployer              string                  `json:"deployer"`
	OracleUpdater         *string                 `json:"oracleUpdater,omitempty"`
	FeeRecipient          string                  `json:"feeRecipient"`
	CollateralToken       *int64                  `json:"collateralToken,omitempty"`
	AssetToStreamingOiCap []PerpDexStreamingOiCap `json:"assetToStreamingOiCap"`
}

// UserStakingSummaryResponse captures staking totals for a user.
type UserStakingSummaryResponse struct {
	Delegated              types.Number `json:"delegated"`
	Undelegated            types.Number `json:"undelegated"`
	TotalPendingWithdrawal types.Number `json:"totalPendingWithdrawal"`
	PendingWithdrawalCount int64        `json:"nPendingWithdrawals"`
}

// UserStakingDelegation represents an individual delegation entry.
type UserStakingDelegation struct {
	Validator            string       `json:"validator"`
	Amount               types.Number `json:"amount"`
	LockedUntilTimestamp types.Time   `json:"lockedUntilTimestamp"`
}

// UserStakingReward represents a single reward accrual.
type UserStakingReward struct {
	Time        types.Time   `json:"time"`
	Source      string       `json:"source"`
	TotalAmount types.Number `json:"totalAmount"`
	Token       *string      `json:"token,omitempty"`
}

// DelegatorHistoryEntry models a staking history log entry.
type DelegatorHistoryEntry struct {
	Time  types.Time            `json:"time"`
	Hash  string                `json:"hash"`
	Delta DelegatorHistoryDelta `json:"delta"`
}

// DelegatorHistoryDelta aggregates the possible staking delta payloads.
type DelegatorHistoryDelta struct {
	Delegate          *DelegatorDelegateDelta          `json:"delegate,omitempty"`
	Deposit           *DelegatorDepositDelta           `json:"cDeposit,omitempty"`
	Withdrawal        *DelegatorWithdrawalDelta        `json:"cWithdraw,omitempty"`
	PendingWithdrawal *DelegatorPendingWithdrawalDelta `json:"pendingWithdrawal,omitempty"`
	Claim             *DelegatorClaimDelta             `json:"claim,omitempty"`
}

// DelegatorDelegateDelta captures delegate and undelegate actions.
type DelegatorDelegateDelta struct {
	Validator    string       `json:"validator"`
	Amount       types.Number `json:"amount"`
	IsUndelegate bool         `json:"isUndelegate"`
}

// DelegatorDepositDelta represents a deposit to the staking contract.
type DelegatorDepositDelta struct {
	Amount types.Number `json:"amount"`
}

// DelegatorWithdrawalDelta denotes a withdrawal from the staking contract.
type DelegatorWithdrawalDelta struct {
	Amount types.Number `json:"amount"`
}

// DelegatorPendingWithdrawalDelta captures pending withdrawal state updates.
type DelegatorPendingWithdrawalDelta struct {
	Amount         types.Number `json:"amount"`
	CompletionTime *types.Time  `json:"completionTime,omitempty"`
}

// DelegatorClaimDelta captures reward claims.
type DelegatorClaimDelta struct {
	Amount    types.Number `json:"amount"`
	Validator *string      `json:"validator,omitempty"`
}

// OrderStatusResponse models the orderStatus info payload.
type OrderStatusResponse struct {
	Status   string             `json:"status"`
	Response *OrderStatusDetail `json:"response,omitempty"`
}

// OrderStatusDetail contains detailed order status entries.
type OrderStatusDetail struct {
	Statuses []OrderStatusEntry `json:"statuses"`
}

// OrderStatusEntry represents a single tracked order state.
type OrderStatusEntry struct {
	Status          string              `json:"status"`
	StatusTimestamp types.Time          `json:"statusTimestamp"`
	Order           *OrderStatusOrder   `json:"order,omitempty"`
	Trigger         *OrderStatusTrigger `json:"trigger,omitempty"`
}

// OrderStatusOrder mirrors the structure provided by the info endpoint.
type OrderStatusOrder struct {
	Coin        string        `json:"coin"`
	Side        string        `json:"side"`
	LimitPrice  types.Number  `json:"limitPx"`
	Size        types.Number  `json:"sz"`
	OrigSize    types.Number  `json:"origSz"`
	OrderID     int64         `json:"oid"`
	Timestamp   types.Time    `json:"timestamp"`
	TriggerPx   *types.Number `json:"triggerPx,omitempty"`
	OrderType   string        `json:"orderType"`
	TimeInForce string        `json:"tif"`
	ReduceOnly  bool          `json:"reduceOnly"`
	ClientOID   *string       `json:"cloid"`
}

// OrderStatusTrigger describes trigger metadata for conditional orders.
type OrderStatusTrigger struct {
	TriggerCondition string       `json:"triggerCondition"`
	TriggerPrice     types.Number `json:"triggerPx"`
}

// ReferralStateResponse captures referral program information for a user.
type ReferralStateResponse struct {
	ReferredBy       *string                      `json:"referredBy"`
	CumulativeVolume types.Number                 `json:"cumVlm"`
	UnclaimedRewards types.Number                 `json:"unclaimedRewards"`
	ClaimedRewards   types.Number                 `json:"claimedRewards"`
	BuilderRewards   types.Number                 `json:"builderRewards"`
	ReferrerState    ReferralProgramState         `json:"referrerState"`
	RewardHistory    []ReferralRewardHistoryEntry `json:"rewardHistory"`
	TokenStates      []ReferralTokenState         `json:"tokenToState"`
}

// ReferralProgramState provides the current referrer program stage.
type ReferralProgramState struct {
	Stage string            `json:"stage"`
	Data  map[string]string `json:"data"`
}

// ReferralRewardHistoryEntry captures individual referral payout events.
type ReferralRewardHistoryEntry struct {
	Time   types.Time   `json:"time"`
	Amount types.Number `json:"amount"`
	Token  string       `json:"token"`
	Type   string       `json:"type"`
	Source string       `json:"source"`
}

// ReferralTokenState summarises referral status for incentivised tokens.
type ReferralTokenState struct {
	Token string            `json:"token"`
	Stage string            `json:"stage"`
	Data  map[string]string `json:"data"`
}

// SubAccount represents a Hyperliquid sub account.
type SubAccount struct {
	Name             string      `json:"name"`
	SubAccountUser   string      `json:"subAccountUser"`
	IsVault          bool        `json:"isVault,omitempty"`
	VaultAddress     *string     `json:"vaultAddress,omitempty"`
	CreatedTimestamp *types.Time `json:"createdTimestamp,omitempty"`
}

// UserMultiSigSigner contains signer metadata for multi-sig accounts.
type UserMultiSigSigner struct {
	Signer     string      `json:"signer"`
	ValidUntil *types.Time `json:"validUntil,omitempty"`
	Weight     *int64      `json:"weight,omitempty"`
	Name       *string     `json:"name,omitempty"`
}

// PerpDeployAuctionStatusResponse describes the current builder deploy auction.
type PerpDeployAuctionStatusResponse struct {
	StartTime       types.Time    `json:"startTimeSeconds"`
	DurationSeconds int64         `json:"durationSeconds"`
	StartGas        types.Number  `json:"startGas"`
	CurrentGas      *types.Number `json:"currentGas"`
	EndGas          *types.Number `json:"endGas"`
}

// UserDexAbstractionResponse details a user's dex abstraction configuration.
type UserDexAbstractionResponse struct {
	User      string      `json:"user"`
	Enabled   bool        `json:"enabled"`
	Timestamp *types.Time `json:"timestamp,omitempty"`
}

// PortfolioPeriod represents performance metrics across a named window.
type PortfolioPeriod struct {
	Period  string
	Metrics PortfolioMetrics
}

// UnmarshalJSON decodes the tuple representation returned by the API.
func (p *PortfolioPeriod) UnmarshalJSON(data []byte) error {
	var payload []json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("hyperliquid: decode portfolio period: %w", err)
	}
	if len(payload) != 2 {
		return errPortfolioPeriodMalformed
	}
	if err := json.Unmarshal(payload[0], &p.Period); err != nil {
		return fmt.Errorf("hyperliquid: decode portfolio period name: %w", err)
	}
	if err := json.Unmarshal(payload[1], &p.Metrics); err != nil {
		return fmt.Errorf("hyperliquid: decode portfolio metrics: %w", err)
	}
	return nil
}

// PortfolioMetrics contains timeseries data for an account window.
type PortfolioMetrics struct {
	AccountValueHistory []PortfolioHistoryPoint `json:"accountValueHistory"`
	PnLHistory          []PortfolioHistoryPoint `json:"pnlHistory"`
	Volume              types.Number            `json:"vlm"`
}

// PortfolioHistoryPoint represents a single (timestamp, value) tuple.
type PortfolioHistoryPoint struct {
	Timestamp types.Time
	Value     types.Number
}

// UnmarshalJSON decodes the tuple representation returned by the API.
func (p *PortfolioHistoryPoint) UnmarshalJSON(data []byte) error {
	var payload []json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("hyperliquid: decode portfolio history point: %w", err)
	}
	if len(payload) != 2 {
		return errHistoryPointMalformed
	}
	if err := json.Unmarshal(payload[0], &p.Timestamp); err != nil {
		return fmt.Errorf("hyperliquid: decode history timestamp: %w", err)
	}
	if err := json.Unmarshal(payload[1], &p.Value); err != nil {
		return fmt.Errorf("hyperliquid: decode history value: %w", err)
	}
	return nil
}

// UserVaultEquity represents a vault balance summary for a user.
type UserVaultEquity struct {
	VaultAddress         string       `json:"vaultAddress"`
	Equity               types.Number `json:"equity"`
	LockedUntilTimestamp *types.Time  `json:"lockedUntilTimestamp,omitempty"`
	PendingWithdrawal    types.Number `json:"pendingWithdrawal,omitempty"`
}

// UserRoleResponse describes the role classification for a user.
type UserRoleResponse struct {
	Role        string  `json:"role"`
	AccountType *string `json:"accountType,omitempty"`
}

// UserRateLimitResponse provides current API limiter utilisation.
type UserRateLimitResponse struct {
	CumulativeVolume types.Number `json:"cumVlm"`
	RequestsUsed     int64        `json:"nRequestsUsed"`
	RequestsCap      int64        `json:"nRequestsCap"`
}

// SpotDeployStateResponse summarises spot deploy auction context.
type SpotDeployStateResponse struct {
	States     []SpotDeployState    `json:"states"`
	GasAuction SpotDeployGasAuction `json:"gasAuction"`
}

// SpotDeployState captures builder deployment state details.
type SpotDeployState struct {
	Name string            `json:"name"`
	Data map[string]string `json:"data"`
}

// SpotDeployGasAuction describes the ongoing gas auction parameters.
type SpotDeployGasAuction struct {
	StartTime       types.Time    `json:"startTimeSeconds"`
	DurationSeconds int64         `json:"durationSeconds"`
	StartGas        types.Number  `json:"startGas"`
	CurrentGas      *types.Number `json:"currentGas"`
	EndGas          *types.Number `json:"endGas"`
}

// ExtraAgent captures metadata about an authorised agent.
type ExtraAgent struct {
	Name       string     `json:"name"`
	Address    string     `json:"address"`
	ValidUntil types.Time `json:"validUntil"`
}
