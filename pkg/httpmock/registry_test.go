package httpmock

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_MatchesAndResponds(t *testing.T) {
	reg := &Registry{}

	reg.Register(
		REST("GET", "/api/v1/repos/owner/repo"),
		StringResponse(http.StatusOK, `{"name":"repo"}`),
	)

	req, _ := http.NewRequest("GET", "https://app.copia.io/api/v1/repos/owner/repo", nil)
	resp, err := reg.RoundTrip(req)
	require.NoError(t, err)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"name":"repo"}`, string(body))
}

func TestRegistry_NoMatch_ReturnsError(t *testing.T) {
	reg := &Registry{}

	req, _ := http.NewRequest("GET", "https://app.copia.io/api/v1/unknown", nil)
	_, err := reg.RoundTrip(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no mock matched")
}

func TestRegistry_MultipleStubs(t *testing.T) {
	reg := &Registry{}

	reg.Register(
		REST("GET", "/api/v1/user"),
		StringResponse(http.StatusOK, `{"login":"john"}`),
	)
	reg.Register(
		REST("POST", "/api/v1/repos"),
		StringResponse(http.StatusCreated, `{"id":1}`),
	)

	req1, _ := http.NewRequest("GET", "https://app.copia.io/api/v1/user", nil)
	resp1, err := reg.RoundTrip(req1)
	require.NoError(t, err)
	body1, _ := io.ReadAll(resp1.Body)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	assert.Contains(t, string(body1), "john")

	req2, _ := http.NewRequest("POST", "https://app.copia.io/api/v1/repos", nil)
	resp2, err := reg.RoundTrip(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)
}

func TestRegistry_Verify_AllCalled(t *testing.T) {
	fakeT := &testing.T{}
	reg := &Registry{}

	reg.Register(
		REST("GET", "/api/v1/user"),
		StringResponse(http.StatusOK, `{}`),
	)

	// Call the stub
	req, _ := http.NewRequest("GET", "https://app.copia.io/api/v1/user", nil)
	_, _ = reg.RoundTrip(req)

	// Should not fail — stub was called
	reg.Verify(fakeT)
	assert.False(t, fakeT.Failed())
}

func TestRegistry_MethodMismatch(t *testing.T) {
	reg := &Registry{}

	reg.Register(
		REST("POST", "/api/v1/user"),
		StringResponse(http.StatusOK, `{}`),
	)

	req, _ := http.NewRequest("GET", "https://app.copia.io/api/v1/user", nil)
	_, err := reg.RoundTrip(req)
	assert.Error(t, err)
}

func TestRegistry_QueryParamToken_ReturnsError(t *testing.T) {
	for _, param := range []string{"token", "access_token"} {
		t.Run(param, func(t *testing.T) {
			reg := &Registry{}
			reg.Register(
				REST("GET", "/api/v1/user"),
				StringResponse(http.StatusOK, `{}`),
			)

			req, _ := http.NewRequest("GET", "https://app.copia.io/api/v1/user?"+param+"=x", nil)
			_, err := reg.RoundTrip(req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "query parameter")
			assert.Contains(t, err.Error(), param)
		})
	}
}

func TestRegistry_HeaderToken_Served(t *testing.T) {
	reg := &Registry{}
	reg.Register(
		REST("GET", "/api/v1/user"),
		StringResponse(http.StatusOK, `{"login":"john"}`),
	)

	req, _ := http.NewRequest("GET", "https://app.copia.io/api/v1/user?page=1&limit=50", nil)
	req.Header.Set("Authorization", "token x")
	resp, err := reg.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
