package hyperliquid

import (
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchange/websocket"
	"github.com/thrasher-corp/gocryptotrader/exchanges/subscription"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// WsSubscriptionRequest subscribes to or unsubscribes from one feed
type WsSubscriptionRequest struct {
	Method       string         `json:"method"`
	Subscription WsSubscription `json:"subscription"`
}

// WsSubscription identifies a feed; the server echoes it in a canonical form, with the user lower case and every
// option present, and each option's zero value is its default, so the request and its echo decode to equal values
type WsSubscription struct {
	Type     string `json:"type"`
	Coin     string `json:"coin,omitempty"`
	Interval string `json:"interval,omitempty"`
	User     string `json:"user,omitempty"`
	// DEX selects a perpetual DEX; empty is the default DEX, whose allMids also include spot and outcome coins
	DEX string `json:"dex,omitempty"`
	// SignificantFigures aggregates an l2Book to 2 to 5 significant figures; zero is full precision
	SignificantFigures uint64 `json:"nSigFigs,omitempty"`
	// Mantissa further aggregates a 5 significant figure l2Book by 1, 2 or 5; zero is unaggregated
	Mantissa uint64 `json:"mantissa,omitempty"`
	// Fast selects the 5 level l2Book pushed about every half second, rather than 20 levels about every 5 seconds
	Fast bool `json:"fast,omitempty"`
	// AggregateByTime combines the partial fills of an order matched within one block
	AggregateByTime bool `json:"aggregateByTime,omitempty"`
	// IgnorePortfolioMargin omits portfolio margin collateral and available amounts from spotState
	IgnorePortfolioMargin bool `json:"ignorePortfolioMargin,omitempty"`
}

// WsSubscriptionResponse acknowledges a subscribe or unsubscribe request
type WsSubscriptionResponse struct {
	Method       string         `json:"method"`
	Subscription WsSubscription `json:"subscription"`
}

// websocketPendingKey identifies a subscription operation awaiting its acknowledgement on one connection
type websocketPendingKey struct {
	authenticated bool
	subscription  WsSubscription
}

// websocketPendingOperation is a subscription operation awaiting its acknowledgement
type websocketPendingOperation struct {
	method        string
	connection    websocket.Connection
	subscription  *subscription.Subscription
	previousState subscription.State
	done          chan error
}

// WsResponse is the outer frame of every server message
type WsResponse struct {
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// WsAllMids contains the mid price of every coin on one perpetual DEX; DEX is empty for the default DEX, which also
// includes spot and outcome coins
type WsAllMids struct {
	DEX  string                  `json:"dex"`
	Mids map[string]types.Number `json:"mids"`
}

// WsNotification contains a notification the frontend displays to a user, such as a fill message
type WsNotification struct {
	Notification string `json:"notification"`
}

// WsWebData3 contains the aggregate account information the frontend displays
type WsWebData3 struct {
	UserState WsWebData3UserState `json:"userState"`
	// PerpetualDEXStates contains one state per perpetual DEX
	PerpetualDEXStates []WsPerpetualDEXState `json:"perpDexStates"`
}

// WsWebData3UserState contains a user's API wallet, ledger and account mode; a vault has no account mode
type WsWebData3UserState struct {
	AgentAddress          string             `json:"agentAddress"`
	AgentValidUntil       types.Time         `json:"agentValidUntil"`
	CumulativeLedger      types.Number       `json:"cumLedger"`
	ServerTime            types.Time         `json:"serverTime"`
	IsVault               bool               `json:"isVault"`
	User                  string             `json:"user"`
	OptOutOfSpotDusting   bool               `json:"optOutOfSpotDusting"`
	DEXAbstractionEnabled bool               `json:"dexAbstractionEnabled"`
	Abstraction           AccountAbstraction `json:"abstraction"`
}

// WsPerpetualDEXState contains one perpetual DEX's vault equity, capped markets and the vaults a user leads
type WsPerpetualDEXState struct {
	TotalVaultEquity            types.Number     `json:"totalVaultEquity"`
	PerpetualsAtOpenInterestCap []string         `json:"perpsAtOpenInterestCap"`
	LeadingVaults               []WsLeadingVault `json:"leadingVaults"`
}

// WsLeadingVault contains a vault a user leads
type WsLeadingVault struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

// WsTWAPStates contains a user's active TWAP orders on one perpetual DEX
type WsTWAPStates struct {
	DEX    string           `json:"dex"`
	User   string           `json:"user"`
	States []TWAPStateEntry `json:"states"`
}

// TWAPStateEntry contains an active TWAP order and its ID
type TWAPStateEntry struct {
	TWAPID uint64
	State  TWAPState
}

// UnmarshalJSON decodes a [twapId, state] tuple
func (t *TWAPStateEntry) UnmarshalJSON(data []byte) error {
	return unmarshalTuple(data, &t.TWAPID, &t.State)
}

// TWAPState contains a TWAP order's parameters and progress
type TWAPState struct {
	Coin             string       `json:"coin"`
	User             string       `json:"user"`
	Side             string       `json:"side"`
	Size             types.Number `json:"sz"`
	ExecutedSize     types.Number `json:"executedSz"`
	ExecutedNotional types.Number `json:"executedNtl"`
	Minutes          uint64       `json:"minutes"`
	ReduceOnly       bool         `json:"reduceOnly"`
	Randomise        bool         `json:"randomize"`
	Timestamp        types.Time   `json:"timestamp"`
	// Trigger is set for a TWAP order that starts once the price crosses a level
	Trigger   *TWAPTrigger `json:"trigger"`
	StopPrice types.Number `json:"stopPx"`
}

// TWAPTrigger contains the price a waiting TWAP order starts at; Above is true when it starts once the price rises
// above Price
type TWAPTrigger struct {
	Price types.Number `json:"px"`
	Above bool         `json:"above"`
}

// WsClearinghouseState contains a user's perpetual margin state on one DEX; DEX is empty for the default DEX
type WsClearinghouseState struct {
	DEX                string                     `json:"dex"`
	User               string                     `json:"user"`
	ClearinghouseState ClearinghouseStateResponse `json:"clearinghouseState"`
}

// WsOpenOrders contains a user's 100 most recent open orders on one perpetual DEX, newest first
type WsOpenOrders struct {
	DEX    string              `json:"dex"`
	User   string              `json:"user"`
	Orders []FrontendOpenOrder `json:"orders"`
}

// WsL2Book contains an L2 book snapshot from the l2Book feed; Fast is set on the 5 level fast book
type WsL2Book struct {
	L2Book
	Fast bool `json:"fast"`
}

// WsOrder contains an order update from the orderUpdates feed, which omits the order type and time in force
type WsOrder struct {
	Order           BasicOrder `json:"order"`
	Status          string     `json:"status"`
	StatusTimestamp types.Time `json:"statusTimestamp"`
}

// WsUserEvent contains one event from the userEvents feed, sent on the user channel; exactly one field is set
type WsUserEvent struct {
	Fills          []Fill            `json:"fills"`
	Funding        *WsUserFunding    `json:"funding"`
	Liquidation    *WsLiquidation    `json:"liquidation"`
	NonUserCancel  []WsNonUserCancel `json:"nonUserCancel"`
	TWAPSliceFills []TWAPSliceFill   `json:"twapSliceFills"`
}

// WsUserFunding contains one funding payment; USDC is negative when the user paid funding
type WsUserFunding struct {
	Time        types.Time   `json:"time"`
	Coin        string       `json:"coin"`
	USDC        types.Number `json:"usdc"`
	SignedSize  types.Number `json:"szi"`
	FundingRate types.Number `json:"fundingRate"`
	// NumberOfSamples is the number of hourly payments a daily aggregate holds, and zero for an hourly payment
	NumberOfSamples uint64 `json:"nSamples"`
}

// WsLiquidation contains a liquidation of a user's account
type WsLiquidation struct {
	LiquidationID              uint64       `json:"lid"`
	Liquidator                 string       `json:"liquidator"`
	LiquidatedUser             string       `json:"liquidated_user"`
	LiquidatedNotionalPosition types.Number `json:"liquidated_ntl_pos"`
	LiquidatedAccountValue     types.Number `json:"liquidated_account_value"`
}

// WsNonUserCancel identifies an order the exchange cancelled
type WsNonUserCancel struct {
	Coin    string `json:"coin"`
	OrderID uint64 `json:"oid"`
}

// WsUserFills contains fills from the userFills feed; the first message is a snapshot of recent fills
type WsUserFills struct {
	IsSnapshot bool   `json:"isSnapshot"`
	User       string `json:"user"`
	Fills      []Fill `json:"fills"`
}

// WsUserFundings contains funding payments from the userFundings feed; the first message is a snapshot of recent
// payments
type WsUserFundings struct {
	IsSnapshot bool            `json:"isSnapshot"`
	User       string          `json:"user"`
	Fundings   []WsUserFunding `json:"fundings"`
}

// WsUserNonFundingLedgerUpdates contains ledger updates other than funding payments; the first message is a snapshot of
// recent updates
type WsUserNonFundingLedgerUpdates struct {
	IsSnapshot              bool               `json:"isSnapshot"`
	User                    string             `json:"user"`
	NonFundingLedgerUpdates []UserLedgerUpdate `json:"nonFundingLedgerUpdates"`
}

// WsPerpetualAssetContext contains a perpetual market's context from the activeAssetCtx feed
type WsPerpetualAssetContext struct {
	Coin    string                `json:"coin"`
	Context PerpetualAssetContext `json:"ctx"`
}

// WsSpotAssetContext contains a spot or outcome market's context from the activeAssetCtx feed, sent on the
// activeSpotAssetCtx channel
type WsSpotAssetContext struct {
	Coin    string           `json:"coin"`
	Context SpotAssetContext `json:"ctx"`
}

// WsUserTWAPSliceFills contains TWAP slice fills from the userTwapSliceFills feed; the first message is a snapshot of
// recent slice fills
type WsUserTWAPSliceFills struct {
	IsSnapshot     bool            `json:"isSnapshot"`
	User           string          `json:"user"`
	TWAPSliceFills []TWAPSliceFill `json:"twapSliceFills"`
}

// WsUserTWAPHistory contains TWAP status changes from the userTwapHistory feed; the first message is a snapshot of
// recent changes
type WsUserTWAPHistory struct {
	IsSnapshot bool               `json:"isSnapshot"`
	User       string             `json:"user"`
	History    []TWAPHistoryEntry `json:"history"`
}

// TWAPHistoryEntry contains a TWAP order's state when its status changed
type TWAPHistoryEntry struct {
	// Time is sent in seconds, unlike the state's millisecond timestamp
	Time   types.Time `json:"time"`
	State  TWAPState  `json:"state"`
	Status TWAPStatus `json:"status"`
	TWAPID uint64     `json:"twapId"`
}

// TWAPStatus contains a TWAP order's status: activated, waitingForTrigger, finished, stopped, terminated or error;
// Description explains an error
type TWAPStatus struct {
	Status      string `json:"status"`
	Description string `json:"description"`
}

// WsBestBidOffer contains a coin's best bid and ask; either is nil when its side of the book is empty
type WsBestBidOffer struct {
	Coin string     `json:"coin"`
	Time types.Time `json:"time"`
	// BestBidOffer contains the bid then the ask
	BestBidOffer [2]*L2Level `json:"bbo"`
}

// WsSpotState contains a user's spot balances
type WsSpotState struct {
	User      string                         `json:"user"`
	SpotState SpotClearinghouseStateResponse `json:"spotState"`
}

// WsAllDEXsClearinghouseState contains a user's perpetual margin state on every DEX
type WsAllDEXsClearinghouseState struct {
	User                string                  `json:"user"`
	ClearinghouseStates []DEXClearinghouseState `json:"clearinghouseStates"`
}

// DEXClearinghouseState contains a user's perpetual margin state on one DEX; DEX is empty for the default DEX
type DEXClearinghouseState struct {
	DEX                string
	ClearinghouseState ClearinghouseStateResponse
}

// UnmarshalJSON decodes a [dex, state] tuple
func (d *DEXClearinghouseState) UnmarshalJSON(data []byte) error {
	return unmarshalTuple(data, &d.DEX, &d.ClearinghouseState)
}

// WsAllDEXsAssetContexts contains every perpetual DEX's asset contexts, each in its DEX's metadata universe order
type WsAllDEXsAssetContexts struct {
	Contexts []DEXAssetContexts `json:"ctxs"`
}

// DEXAssetContexts contains one perpetual DEX's asset contexts; DEX is empty for the default DEX
type DEXAssetContexts struct {
	DEX           string
	AssetContexts []PerpetualAssetContext
}

// UnmarshalJSON decodes a [dex, contexts] tuple
func (d *DEXAssetContexts) UnmarshalJSON(data []byte) error {
	return unmarshalTuple(data, &d.DEX, &d.AssetContexts)
}

// WsOutcomeMetaUpdate contains one outcome or question change; exactly one field is set
type WsOutcomeMetaUpdate struct {
	OutcomeCreated  *OutcomeSpecification `json:"outcomeCreated"`
	OutcomeSettled  *uint64               `json:"outcomeSettled"`
	QuestionUpdated *OutcomeQuestion      `json:"questionUpdated"`
	QuestionSettled *uint64               `json:"questionSettled"`
}

// WsFastAssetContext contains a coin's changed mark and mid prices from the fastAssetCtxs feed; a field that is not set
// is unchanged, and MidPriceSet with a zero MidPrice means the book has no mid price
type WsFastAssetContext struct {
	MarkPrice    types.Number
	MidPrice     types.Number
	MarkPriceSet bool
	MidPriceSet  bool
}

// UnmarshalJSON records which prices are present, so updates can be merged into the previous snapshot
func (w *WsFastAssetContext) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if value, ok := fields["markPx"]; ok {
		w.MarkPriceSet = true
		if err := json.Unmarshal(value, &w.MarkPrice); err != nil {
			return err
		}
	}
	if value, ok := fields["midPx"]; ok {
		w.MidPriceSet = true
		if err := json.Unmarshal(value, &w.MidPrice); err != nil {
			return err
		}
	}
	return nil
}

// WsPostRequest sends an info request or a signed action over the websocket
type WsPostRequest struct {
	Method string `json:"method"`
	// ID must be unique among the client's outstanding requests, as Hyperliquid does not check it
	ID      uint64            `json:"id"`
	Request WsPostRequestBody `json:"request"`
}

// WsPostRequestBody contains a post request's type, info or action, and its payload, an info request body or a signed
// action request
type WsPostRequestBody struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// WsPostResponse contains a post request's ID and outcome
type WsPostResponse struct {
	ID       uint64             `json:"id"`
	Response WsPostResponseBody `json:"response"`
}

// WsPostResponseBody contains a post response's type, info, action or error, and its payload: a WsPostInfoPayload for
// info, an exchange action response for action and an HTTP status for error
type WsPostResponseBody struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// WsPostInfoPayload contains an info request's type and its response, as the info endpoint returns it
type WsPostInfoPayload struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
