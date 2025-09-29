package accounts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestIsEmpty(t *testing.T) {
	t.Parallel()
	var c *Credentials
	assert.True(t, c.IsEmpty(), "IsEmpty should return true for nil credentials")
	c = new(Credentials)
	assert.True(t, c.IsEmpty(), "IsEmpty should return true for empty credentials")

	c.SubAccount = "woow"
	assert.False(t, c.IsEmpty(), "IsEmpty should return false when sub account set")
}

func TestParseCredentialsMetadata(t *testing.T) {
	t.Parallel()
	_, err := ParseCredentialsMetadata(t.Context(), nil)
	require.ErrorIs(t, err, errMetaDataIsNil)

	_, err = ParseCredentialsMetadata(t.Context(), metadata.MD{})
	require.NoError(t, err)

	ctx := metadata.AppendToOutgoingContext(t.Context(),
		string(ContextCredentialsFlag), "wow", string(ContextCredentialsFlag), "wow2")
	nortyMD, _ := metadata.FromOutgoingContext(ctx)

	_, err = ParseCredentialsMetadata(t.Context(), nortyMD)
	require.ErrorIs(t, err, errInvalidCredentialMetaDataLength)

	ctx = metadata.AppendToOutgoingContext(t.Context(),
		string(ContextCredentialsFlag), "brokenstring")
	nortyMD, _ = metadata.FromOutgoingContext(ctx)

	_, err = ParseCredentialsMetadata(t.Context(), nortyMD)
	require.ErrorIs(t, err, errMissingInfo)

	beforeCreds := Credentials{
		Key:             "superkey",
		Secret:          "supersecret",
		SubAccount:      "supersub",
		ClientID:        "superclient",
		PEMKey:          "superpem",
		OneTimePassword: "superOneTimePasssssss",
	}

	flag, outGoing := beforeCreds.GetMetaData()
	ctx = metadata.AppendToOutgoingContext(t.Context(), flag, outGoing)
	lovelyMD, _ := metadata.FromOutgoingContext(ctx)

	ctx, err = ParseCredentialsMetadata(t.Context(), lovelyMD)
	require.NoError(t, err)

	store, ok := ctx.Value(ContextCredentialsFlag).(*ContextCredentialsStore)
	require.True(t, ok, "ParseCredentialsMetadata must populate ContextCredentialsStore")

	afterCreds := store.Get()

	assert.Equal(t, "superkey", afterCreds.Key, "ParseCredentialsMetadata should restore key")
	assert.Equal(t, "supersecret", afterCreds.Secret, "ParseCredentialsMetadata should restore secret")
	assert.Equal(t, "supersub", afterCreds.SubAccount, "ParseCredentialsMetadata should restore sub account")
	assert.Equal(t, "superclient", afterCreds.ClientID, "ParseCredentialsMetadata should restore client id")
	assert.Equal(t, "superpem", afterCreds.PEMKey, "ParseCredentialsMetadata should restore pem key")
	assert.Equal(t, "superOneTimePasssssss", afterCreds.OneTimePassword, "ParseCredentialsMetadata should restore otp")

	// subaccount override
	subaccount := Credentials{
		SubAccount: "supersub",
	}

	flag, outGoing = subaccount.GetMetaData()
	ctx = metadata.AppendToOutgoingContext(t.Context(), flag, outGoing)
	lovelyMD, _ = metadata.FromOutgoingContext(ctx)

	ctx, err = ParseCredentialsMetadata(t.Context(), lovelyMD)
	require.NoError(t, err)

	sa, ok := ctx.Value(ContextSubAccountFlag).(string)
	require.True(t, ok, "ParseCredentialsMetadata must populate sub account flag")
	assert.Equal(t, "supersub", sa, "ParseCredentialsMetadata should restore sub account override")
}

func TestGetInternal(t *testing.T) {
	t.Parallel()
	flag, store := (&Credentials{}).getInternal()
	assert.Equal(t, contextCredential(""), flag, "getInternal should return empty flag when credentials empty")
	assert.Nil(t, store, "getInternal should return nil store when credentials empty")
	flag, store = (&Credentials{Key: "wow"}).getInternal()
	assert.Equal(t, ContextCredentialsFlag, flag, "getInternal should return credentials flag when key present")
	require.NotNil(t, store, "getInternal must return store when key present")
	assert.Equal(t, "wow", store.Get().Key, "getInternal should set key in store")
}

func TestString(t *testing.T) {
	t.Parallel()
	creds := Credentials{}
	assert.Equal(t, "Key:[...] SubAccount:[] ClientID:[]", creds.String(), "String should mask empty credentials")

	creds.Key = "12345678910111234"
	creds.SubAccount = "sub"
	creds.ClientID = "client"

	assert.Equal(t, "Key:[1234567891011123...] SubAccount:[sub] ClientID:[client]", creds.String(), "String should mask credential values")
}

func TestCredentialsEqual(t *testing.T) {
	t.Parallel()
	var this, that *Credentials
	assert.False(t, this.Equal(that), "Equal should return false for nil credentials")
	this = &Credentials{}
	assert.False(t, this.Equal(that), "Equal should return false when other nil")
	that = &Credentials{Key: "1337"}
	assert.False(t, this.Equal(that), "Equal should return false when keys differ")
	this.Key = "1337"
	assert.True(t, this.Equal(that), "Equal should return true when only keys match")
	this.ClientID = "1337"
	assert.False(t, this.Equal(that), "Equal should return false when client ids differ")
	that.ClientID = "1337"
	assert.True(t, this.Equal(that), "Equal should return true when keys and client ids match")
	this.SubAccount = "someSub"
	assert.False(t, this.Equal(that), "Equal should return false when sub accounts differ")
	that.SubAccount = "someSub"
	assert.True(t, this.Equal(that), "Equal should return true when all credentials match")
}
