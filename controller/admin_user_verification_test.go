package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupAdminUserTest promotes the enrolled operator to root and creates a
// managed common user for the administrative endpoints to act on.
func setupAdminUserTest(t *testing.T) (*model.User, service.AuthIdentity, *model.User) {
	t.Helper()
	operator, identity := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.Model(operator).Update("role", common.RoleRootUser).Error)
	require.NoError(t, model.PublishUserAuthCache(operator.Id))
	password, err := common.Password2Hash("managed-password")
	require.NoError(t, err)
	target := &model.User{Username: "managed-user", Password: password, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "managed-aff", GitHubId: "managed-github"}
	require.NoError(t, model.DB.Create(target).Error)
	return operator, identity, target
}

// adminUserRequest drives an admin user-management handler with the operator
// context that AdminAuth would have populated for a dashboard session.
func adminUserRequest(method, path, body, proof string, identity service.AuthIdentity, role int, params gin.Params, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if proof != "" {
		c.Request.Header.Set("X-Security-Proof", proof)
	}
	c.Params = params
	c.Set("id", identity.UserID)
	c.Set("username", "enrollment-user")
	c.Set("role", role)
	c.Set("session_id", identity.SessionID)
	c.Set("auth_version", identity.UserAuthVersion)
	c.Set("session_version", identity.SessionVersion)
	handler(c)
	return response
}

func TestAdminUserRiskOperationsRequireProofBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name, method, path string
		body               func(target *model.User) string
		params             func(target *model.User) gin.Params
		handler            gin.HandlerFunc
		unchanged          func(t *testing.T, target *model.User)
	}{
		{
			name: "hard delete", method: http.MethodDelete, path: "/api/user/:id", handler: DeleteUser,
			params: func(target *model.User) gin.Params { return gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}} },
			unchanged: func(t *testing.T, target *model.User) {
				_, err := model.GetUserById(target.Id, false)
				assert.NoError(t, err)
			},
		},
		{
			name: "manage disable", method: http.MethodPost, path: "/api/user/manage", handler: ManageUser,
			body: func(target *model.User) string { return fmt.Sprintf(`{"id":%d,"action":"disable"}`, target.Id) },
			unchanged: func(t *testing.T, target *model.User) {
				stored, err := model.GetUserById(target.Id, false)
				require.NoError(t, err)
				assert.Equal(t, common.UserStatusEnabled, stored.Status)
			},
		},
		{
			name: "manage promote", method: http.MethodPost, path: "/api/user/manage", handler: ManageUser,
			body: func(target *model.User) string { return fmt.Sprintf(`{"id":%d,"action":"promote"}`, target.Id) },
			unchanged: func(t *testing.T, target *model.User) {
				stored, err := model.GetUserById(target.Id, false)
				require.NoError(t, err)
				assert.Equal(t, common.RoleCommonUser, stored.Role)
			},
		},
		{
			name: "manage delete", method: http.MethodPost, path: "/api/user/manage", handler: ManageUser,
			body: func(target *model.User) string { return fmt.Sprintf(`{"id":%d,"action":"delete"}`, target.Id) },
			unchanged: func(t *testing.T, target *model.User) {
				_, err := model.GetUserById(target.Id, false)
				assert.NoError(t, err)
			},
		},
		{
			name: "password reset", method: http.MethodPut, path: "/api/user/", handler: UpdateUser,
			body: func(target *model.User) string {
				return fmt.Sprintf(`{"id":%d,"username":"managed-user","display_name":"Managed","password":"replacement-pass"}`, target.Id)
			},
			unchanged: func(t *testing.T, target *model.User) {
				stored, err := model.GetUserById(target.Id, true)
				require.NoError(t, err)
				assert.True(t, common.ValidatePasswordAndHash("managed-password", stored.Password))
			},
		},
		{
			name: "admin permission matrix", method: http.MethodPut, path: "/api/user/", handler: UpdateUser,
			body: func(target *model.User) string {
				return fmt.Sprintf(`{"id":%d,"username":"managed-user","display_name":"Managed","admin_permissions":{}}`, target.Id)
			},
			unchanged: func(t *testing.T, target *model.User) {
				stored, err := model.GetUserById(target.Id, false)
				require.NoError(t, err)
				assert.Equal(t, "", stored.DisplayName)
			},
		},
		{
			name: "create administrator", method: http.MethodPost, path: "/api/user/", handler: CreateUser,
			body: func(*model.User) string { return `{"username":"new-admin","password":"admin-password-1","role":10}` },
			unchanged: func(t *testing.T, _ *model.User) {
				var count int64
				require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "new-admin").Count(&count).Error)
				assert.Zero(t, count)
			},
		},
		{
			name: "passkey reset", method: http.MethodDelete, path: "/api/user/:id/reset_passkey", handler: AdminResetPasskey,
			params: func(target *model.User) gin.Params { return gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}} },
			unchanged: func(t *testing.T, target *model.User) {
				_, err := model.GetPasskeyByUserID(target.Id)
				assert.NoError(t, err)
			},
		},
		{
			name: "two-factor disable", method: http.MethodDelete, path: "/api/user/:id/2fa", handler: AdminDisable2FA,
			params: func(target *model.User) gin.Params { return gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}} },
			unchanged: func(t *testing.T, target *model.User) {
				twoFA, err := model.GetTwoFAByUserId(target.Id)
				require.NoError(t, err)
				assert.True(t, twoFA.IsEnabled)
			},
		},
		{
			name: "built-in binding clear", method: http.MethodDelete, path: "/api/user/:id/bindings/:binding_type", handler: AdminClearUserBinding,
			params: func(target *model.User) gin.Params {
				return gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}, {Key: "binding_type", Value: "github"}}
			},
			unchanged: func(t *testing.T, target *model.User) {
				stored, err := model.GetUserById(target.Id, false)
				require.NoError(t, err)
				assert.Equal(t, "managed-github", stored.GitHubId)
			},
		},
		{
			name: "custom oauth unbind", method: http.MethodDelete, path: "/api/user/:id/oauth/bindings/:provider_id", handler: UnbindCustomOAuthByAdmin,
			params: func(target *model.User) gin.Params {
				return gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}, {Key: "provider_id", Value: "7"}}
			},
			unchanged: func(t *testing.T, target *model.User) {
				bindings, err := model.GetUserOAuthBindingsByUserId(target.Id)
				require.NoError(t, err)
				assert.Len(t, bindings, 1)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, identity, target := setupAdminUserTest(t)
			require.NoError(t, model.DB.Create(&model.PasskeyCredential{UserID: target.Id, CredentialID: "managed-passkey", PublicKey: "public-key"}).Error)
			require.NoError(t, model.DB.Create(&model.TwoFA{UserId: target.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
			require.NoError(t, model.DB.Create(&model.UserOAuthBinding{UserId: target.Id, ProviderId: 7, ProviderUserId: "provider-user"}).Error)
			var body string
			if test.body != nil {
				body = test.body(target)
			}
			var params gin.Params
			if test.params != nil {
				params = test.params(target)
			}
			response := adminUserRequest(test.method, test.path, body, "", identity, common.RoleRootUser, params, test.handler)
			var result securityEnrollmentResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Equal(t, "SECURITY_PROOF_REQUIRED", result.Code)
			test.unchanged(t, target)
			var flows int64
			require.NoError(t, model.DB.Model(&model.AuthFlow{}).Count(&flows).Error)
			assert.Zero(t, flows)
		})
	}
}

// An administrator (not root) exercises the routine paths without touching the
// Casbin policy store, which the security fixture does not provision.
func TestAdminUserRoutineEditsDoNotRequireProof(t *testing.T) {
	_, identity, target := setupAdminUserTest(t)
	response := adminUserRequest(http.MethodPut, "/api/user/", fmt.Sprintf(`{"id":%d,"username":"managed-user","display_name":"Renamed","group":"default"}`, target.Id), "", identity, common.RoleAdminUser, nil, UpdateUser)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	stored, err := model.GetUserById(target.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", stored.DisplayName)
	assert.True(t, common.ValidatePasswordAndHash("managed-password", stored.Password))

	response = adminUserRequest(http.MethodPost, "/api/user/", `{"username":"new-member","password":"member-password-1","role":1}`, "", identity, common.RoleAdminUser, nil, CreateUser)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "new-member").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestCreateUserRejectsNonStandardRole(t *testing.T) {
	_, identity, _ := setupAdminUserTest(t)
	for i, test := range []struct {
		name         string
		role         int
		operatorRole int
	}{
		{"undefined role between leader and admin", 6, common.RoleAdminUser},
		{"negative", -1, common.RoleAdminUser},
		{"between admin and root", 99, common.RoleRootUser},
	} {
		t.Run(test.name, func(t *testing.T) {
			username := fmt.Sprintf("odd-role-%d", i)
			body := fmt.Sprintf(`{"username":%q,"password":"member-password-1","role":%d}`, username, test.role)
			response := adminUserRequest(http.MethodPost, "/api/user/", body, "", identity, test.operatorRole, nil, CreateUser)
			var result securityEnrollmentResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, http.StatusOK, response.Code)
			assert.False(t, result.Success)
			assert.Empty(t, result.Code)
			var count int64
			require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", username).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestAdminUserProofIsBoundToTargetAndActionAndConsumedOnce(t *testing.T) {
	_, identity, target := setupAdminUserTest(t)
	other := &model.User{Username: "other-user", Password: target.Password, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "other-aff"}
	require.NoError(t, model.DB.Create(other).Error)
	disableTarget := service.VerificationOperation{Scope: service.VerificationScopeAdminUserManage, Context: []byte(fmt.Sprintf(`{"user_id":%d,"action":"disable"}`, target.Id))}
	proof := issueSecurityEnrollmentProof(t, identity, disableTarget, service.VerificationMethodPassword)

	for _, test := range []struct {
		name, body, code string
		params           gin.Params
		handler          gin.HandlerFunc
	}{
		{"different action", fmt.Sprintf(`{"id":%d,"action":"enable"}`, target.Id), "SECURITY_PROOF_CONTEXT_MISMATCH", nil, ManageUser},
		{"different user", fmt.Sprintf(`{"id":%d,"action":"disable"}`, other.Id), "SECURITY_PROOF_CONTEXT_MISMATCH", nil, ManageUser},
		{"different scope", "", "SECURITY_PROOF_SCOPE_MISMATCH", gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}}, DeleteUser},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := adminUserRequest(http.MethodPost, "/api/user/manage", test.body, proof, identity, common.RoleRootUser, test.params, test.handler)
			var result securityEnrollmentResponse
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Equal(t, test.code, result.Code)
		})
	}
	for _, id := range []int{target.Id, other.Id} {
		stored, err := model.GetUserById(id, false)
		require.NoError(t, err)
		assert.Equal(t, common.UserStatusEnabled, stored.Status)
	}

	response := adminUserRequest(http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"disable"}`, target.Id), proof, identity, common.RoleRootUser, nil, ManageUser)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	stored, err := model.GetUserById(target.Id, false)
	require.NoError(t, err)
	assert.Equal(t, common.UserStatusDisabled, stored.Status)
	var audit model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "user.manage").Last(&audit).Error)
	require.NotNil(t, audit.Other.Op)
	verificationMethod, err := common.Marshal(audit.Other.Op.Params["verification_method"])
	require.NoError(t, err)
	assert.JSONEq(t, `"password"`, string(verificationMethod))
	targetUserID, err := common.Marshal(audit.Other.Op.Params["target_user_id"])
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprint(target.Id), string(targetUserID))
	auditJSON, err := common.Marshal(audit)
	require.NoError(t, err)
	assert.NotContains(t, string(auditJSON), proof)

	response = adminUserRequest(http.MethodPost, "/api/user/manage", fmt.Sprintf(`{"id":%d,"action":"disable"}`, target.Id), proof, identity, common.RoleRootUser, nil, ManageUser)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "SECURITY_PROOF_CONSUMED", result.Code)
}

func TestAdminUserBindingProofDistinguishesBuiltInAndCustomBindings(t *testing.T) {
	_, identity, target := setupAdminUserTest(t)
	require.NoError(t, model.DB.Create(&model.UserOAuthBinding{UserId: target.Id, ProviderId: 7, ProviderUserId: "provider-user"}).Error)
	custom := service.VerificationOperation{Scope: service.VerificationScopeAdminUserBindingClear, Context: []byte(fmt.Sprintf(`{"user_id":%d,"provider_id":7}`, target.Id))}
	proof := issueSecurityEnrollmentProof(t, identity, custom, service.VerificationMethodPassword)
	params := gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}, {Key: "binding_type", Value: "github"}}
	response := adminUserRequest(http.MethodDelete, "/api/user/:id/bindings/:binding_type", "", proof, identity, common.RoleRootUser, params, AdminClearUserBinding)
	var result securityEnrollmentResponse
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.Equal(t, "SECURITY_PROOF_CONTEXT_MISMATCH", result.Code)
	stored, err := model.GetUserById(target.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "managed-github", stored.GitHubId)

	params = gin.Params{{Key: "id", Value: fmt.Sprint(target.Id)}, {Key: "provider_id", Value: "7"}}
	response = adminUserRequest(http.MethodDelete, "/api/user/:id/oauth/bindings/:provider_id", "", proof, identity, common.RoleRootUser, params, UnbindCustomOAuthByAdmin)
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	bindings, err := model.GetUserOAuthBindingsByUserId(target.Id)
	require.NoError(t, err)
	assert.Empty(t, bindings)
	var audit model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "user.binding_clear").Last(&audit).Error)
	require.NotNil(t, audit.Other.Op)
	providerID, err := common.Marshal(audit.Other.Op.Params["provider_id"])
	require.NoError(t, err)
	assert.JSONEq(t, "7", string(providerID))
}

func TestAdminUserVerificationPolicy(t *testing.T) {
	t.Run("requires an administrator", func(t *testing.T) {
		_, identity := setupSecurityEnrollmentTest(t)
		_, err := service.GetVerificationRequirements(identity, service.VerificationScopeAdminUserDelete)
		assert.ErrorIs(t, err, service.ErrVerificationForbidden)
	})
	t.Run("falls back to the password without a second factor", func(t *testing.T) {
		_, identity, _ := setupAdminUserTest(t)
		requirements, err := service.GetVerificationRequirements(identity, service.VerificationScopeAdminUserDelete)
		require.NoError(t, err)
		assert.Equal(t, []service.VerificationMethodOption{{Method: service.VerificationMethodPassword, Available: true}}, requirements.Methods)
	})
	t.Run("prefers an enrolled second factor", func(t *testing.T) {
		operator, identity, _ := setupAdminUserTest(t)
		require.NoError(t, model.DB.Create(&model.TwoFA{UserId: operator.Id, Secret: "JBSWY3DPEHPK3PXP", IsEnabled: true}).Error)
		requirements, err := service.GetVerificationRequirements(identity, service.VerificationScopeAdminUserManage)
		require.NoError(t, err)
		assert.Equal(t, []service.VerificationMethodOption{{Method: service.VerificationMethodTwoFA, Available: true}}, requirements.Methods)
	})
	t.Run("a legacy token cannot present a proof", func(t *testing.T) {
		operator, identity, target := setupAdminUserTest(t)
		require.NoError(t, model.DB.Model(operator).Update("access_token", "legacy-admin-token").Error)
		proof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{Scope: service.VerificationScopeAdminUserDelete, Context: []byte(fmt.Sprintf(`{"user_id":%d}`, target.Id))}, service.VerificationMethodPassword)
		response := accessTokenRequest(newAccessTokenTestRouter(), http.MethodDelete, fmt.Sprintf("/api/user/%d", target.Id), "legacy-admin-token", proof, "")
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Equal(t, "SECURITY_PROOF_INVALID", decodeSecurityEnrollmentResponse(t, response).Code)
		_, err := model.GetUserById(target.Id, false)
		assert.NoError(t, err)
	})
	t.Run("a user:write token completes the deletion on its own proof", func(t *testing.T) {
		operator, _, target := setupAdminUserTest(t)
		require.NoError(t, model.DB.AutoMigrate(&model.ExternalIdentityClaim{}, &model.Token{}))
		raw, _ := createScopedAccessToken(t, operator.Id, 0, "user:write")
		router := newAccessTokenTestRouter()
		response := accessTokenRequest(router, http.MethodPost, "/api/verify", raw, "", fmt.Sprintf(`{"scope":"admin.user.delete","method":"password","password":"enrollment-password","context":{"user_id":%d}}`, target.Id))
		body := decodeSecurityEnrollmentResponse(t, response)
		require.True(t, body.Success, body.Message)
		var proof service.SecurityProof
		require.NoError(t, common.Unmarshal(body.Data, &proof))

		response = accessTokenRequest(router, http.MethodDelete, fmt.Sprintf("/api/user/%d", target.Id), raw, proof.ProofToken, "")
		body = decodeSecurityEnrollmentResponse(t, response)
		require.True(t, body.Success, body.Message)
		_, err := model.GetUserById(target.Id, true)
		assert.Error(t, err)
		var audit model.AuditLog
		require.NoError(t, model.LOG_DB.Where("action = ?", "user.delete").Last(&audit).Error)
		assert.Equal(t, "access_token", audit.AuthMethod)
		auditJSON, err := common.Marshal(audit)
		require.NoError(t, err)
		for _, secret := range []string{raw, proof.ProofToken, "enrollment-password"} {
			assert.NotContains(t, string(auditJSON), secret)
		}
	})
	t.Run("a user:read token cannot obtain a proof", func(t *testing.T) {
		operator, _, target := setupAdminUserTest(t)
		raw, _ := createScopedAccessToken(t, operator.Id, 0, "user:read")
		response := accessTokenRequest(newAccessTokenTestRouter(), http.MethodPost, "/api/verify", raw, "", fmt.Sprintf(`{"scope":"admin.user.delete","method":"password","password":"enrollment-password","context":{"user_id":%d}}`, target.Id))
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Equal(t, "SECURITY_ACTION_FORBIDDEN", decodeSecurityEnrollmentResponse(t, response).Code)
		var flows int64
		require.NoError(t, model.DB.Model(&model.AuthFlow{}).Count(&flows).Error)
		assert.Zero(t, flows)
	})
}

// The list and deletion paths share the real database fixture so the contract
// is exercised against each supported engine, including a separate audit DB.
func TestAdminListsAndBatchDeletionDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("real database DSN not configured")
			}
			t.Setenv("TEST_SECURITY_DIALECT", dialect)
			operator, identity, target := setupAdminUserTest(t)
			require.NoError(t, model.DB.AutoMigrate(&model.Token{}, &model.ExternalIdentityClaim{}))
			other := &model.User{Username: "other-root", AffCode: "other-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AuthVersion: 1}
			require.NoError(t, model.DB.Create(other).Error)
			deleted := &model.User{Username: "deleted-member", AffCode: "deleted-member", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1}
			require.NoError(t, model.DB.Create(deleted).Error)
			require.NoError(t, model.DB.Delete(deleted).Error)
			require.NoError(t, model.DB.Model(target).Update("email", "managed@example.com").Error)
			key := seedToken(t, model.DB, target.Id, "shared-name", "managed-secret-key")
			rootKey := seedToken(t, model.DB, operator.Id, "shared-name", "root-secret-key")
			seedToken(t, model.DB, deleted.Id, "deleted-key", "deleted-secret-key")
			bundle, err := service.CreateLoginSession(target.Id, "password", "127.0.0.1", "batch-test")
			require.NoError(t, err)
			_, accessToken := createScopedAccessToken(t, target.Id, 0, "user:read")
			targetIdentity, err := service.ParseAccessToken(bundle.AccessToken)
			require.NoError(t, err)
			issueSecurityEnrollmentProof(t, targetIdentity, service.VerificationOperation{Scope: service.VerificationScopePasswordChange}, service.VerificationMethodPassword)
			require.NoError(t, model.DB.Create(&model.ExternalIdentityClaim{Provider: "github", Subject: target.GitHubId, UserId: target.Id}).Error)

			for _, endpoint := range []struct {
				path    string
				handler gin.HandlerFunc
			}{
				{"/api/token/?user_id=0", GetAllTokens},
				{"/api/token/search?user_id=0&keyword=shared-name", SearchTokens},
			} {
				response := adminUserRequest(http.MethodGet, endpoint.path, "", "", identity, common.RoleRootUser, nil, endpoint.handler)
				var page struct {
					Success bool
					Data    struct {
						Items []struct {
							UserID   int    `json:"user_id"`
							Username string `json:"username"`
							Key      string `json:"key"`
						} `json:"items"`
					}
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &page))
				require.True(t, page.Success)
				owners := map[int]string{}
				for _, item := range page.Data.Items {
					owners[item.UserID] = item.Username
				}
				assert.Equal(t, target.Username, owners[target.Id])
				assert.Equal(t, operator.Username, owners[operator.Id])
				assert.NotContains(t, response.Body.String(), key.Key)
				assert.NotContains(t, response.Body.String(), rootKey.Key)
				response = adminUserRequest(http.MethodGet, endpoint.path, "", "", identity, common.RoleCommonUser, nil, endpoint.handler)
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &page))
				require.True(t, page.Success)
				require.Len(t, page.Data.Items, 1)
				assert.Equal(t, operator.Username, page.Data.Items[0].Username)
			}

			for _, endpoint := range []struct {
				path    string
				handler gin.HandlerFunc
			}{
				{"/api/user/", GetAllUsers}, {"/api/user/search?keyword=managed", SearchUsers},
			} {
				response := adminUserRequest(http.MethodGet, endpoint.path, "", "", identity, common.RoleRootUser, nil, endpoint.handler)
				assert.Contains(t, response.Body.String(), "managed@example.com")
			}

			// Non-root batches stay owner-scoped; root can remove other owners' keys.
			probeKey := seedToken(t, model.DB, target.Id, "batch-probe", "batch-probe-secret")
			probeBody := fmt.Sprintf(`{"ids":[%d]}`, probeKey.Id)
			deniedToken := adminUserRequest(http.MethodPost, "/api/token/batch", probeBody, "", identity, common.RoleCommonUser, nil, DeleteTokenBatch)
			assert.Contains(t, deniedToken.Body.String(), `"data":0`)
			rootToken := adminUserRequest(http.MethodPost, "/api/token/batch", probeBody, "", identity, common.RoleRootUser, nil, DeleteTokenBatch)
			assert.Contains(t, rootToken.Body.String(), `"data":1`)
			var remainingKey int64
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", probeKey.Id).Count(&remainingKey).Error)
			assert.Zero(t, remainingKey)

			body := fmt.Sprintf(`{"ids":[%d,%d,%d,%d,%d,%d]}`, other.Id, target.Id, operator.Id, deleted.Id, target.Id, 999999)
			operation := service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatchDelete, Context: []byte(fmt.Sprintf(`{"user_ids":[%d,%d,%d,%d,999999]}`, operator.Id, target.Id, deleted.Id, other.Id))}
			missing := adminUserRequest(http.MethodPost, "/api/user/batch/delete", body, "", identity, common.RoleRootUser, nil, DeleteUserBatch)
			assert.Contains(t, missing.Body.String(), "SECURITY_PROOF_REQUIRED")
			for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser} {
				denied := adminUserRequest(http.MethodPost, "/api/user/batch/delete", body, "", identity, role, nil, DeleteUserBatch)
				assert.Contains(t, denied.Body.String(), `"success":false`)
			}
			for _, invalid := range []string{`{"ids":[]}`, `{"ids":[0]}`, `{"ids":[-1]}`, `{"ids":"all"}`} {
				denied := adminUserRequest(http.MethodPost, "/api/user/batch/delete", invalid, "", identity, common.RoleRootUser, nil, DeleteUserBatch)
				assert.Contains(t, denied.Body.String(), `"success":false`)
			}
			wrongOperation := service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatchDelete, Context: []byte(fmt.Sprintf(`{"user_ids":[%d]}`, target.Id))}
			wrongProof := issueSecurityEnrollmentProof(t, identity, wrongOperation, service.VerificationMethodPassword)
			mismatch := adminUserRequest(http.MethodPost, "/api/user/batch/delete", body, wrongProof, identity, common.RoleRootUser, nil, DeleteUserBatch)
			assert.Contains(t, mismatch.Body.String(), "SECURITY_PROOF_CONTEXT_MISMATCH")
			expiredProof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
			require.NoError(t, model.DB.Model(&model.AuthFlow{}).Where("user_id = ? AND consumed_at IS NULL", operator.Id).Update("expires_at", time.Now().Add(-time.Minute)).Error)
			expired := adminUserRequest(http.MethodPost, "/api/user/batch/delete", body, expiredProof, identity, common.RoleRootUser, nil, DeleteUserBatch)
			assert.Contains(t, expired.Body.String(), "SECURITY_PROOF_EXPIRED")
			_, err = model.GetUserById(target.Id, false)
			require.NoError(t, err)

			// A failure at the final user delete must roll back credentials too.
			require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register("batch_delete_failure", func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.ModelType == reflect.TypeFor[model.User]() {
					tx.AddError(errors.New("injected user delete failure"))
				}
			}))
			_, err = model.BatchDeleteUsers([]int{target.Id, deleted.Id})
			require.Error(t, err)
			require.NoError(t, model.DB.Callback().Delete().Remove("batch_delete_failure"))
			for _, entity := range []any{&model.UserSession{}, &model.UserAccessToken{}, &model.Token{}} {
				var remaining int64
				require.NoError(t, model.DB.Model(entity).Where("user_id = ?", target.Id).Count(&remaining).Error)
				assert.Positive(t, remaining)
			}

			proof := issueSecurityEnrollmentProof(t, identity, operation, service.VerificationMethodPassword)
			response := adminUserRequest(http.MethodPost, "/api/user/batch/delete", body, proof, identity, common.RoleRootUser, nil, DeleteUserBatch)
			assert.Contains(t, response.Body.String(), `"success":true`)
			assert.Contains(t, response.Body.String(), `"deleted":2`)
			assert.Contains(t, response.Body.String(), `"skipped":3`)
			for _, entity := range []any{&model.User{}, &model.Token{}} {
				column := "user_id"
				if _, ok := entity.(*model.User); ok {
					column = "id"
				}
				var remaining int64
				require.NoError(t, model.DB.Unscoped().Model(entity).Where(column+" IN ?", []int{target.Id, deleted.Id}).Count(&remaining).Error)
				assert.Zero(t, remaining)
			}
			for _, entity := range []any{&model.UserSession{}, &model.UserAccessToken{}, &model.AuthFlow{}, &model.ExternalIdentityClaim{}} {
				var remaining int64
				require.NoError(t, model.DB.Model(entity).Where("user_id = ?", target.Id).Count(&remaining).Error)
				assert.Zero(t, remaining)
			}
			deletedIdentity, err := service.ParseAccessToken(bundle.AccessToken)
			if err == nil {
				err = service.ValidateStepUpIdentity(deletedIdentity)
			}
			assert.Error(t, err)
			var tokenCount int64
			require.NoError(t, model.DB.Model(&model.UserAccessToken{}).Where("id = ?", accessToken.Id).Count(&tokenCount).Error)
			assert.Zero(t, tokenCount)
			var rootCount int64
			require.NoError(t, model.DB.Model(&model.User{}).Where("id IN ?", []int{operator.Id, other.Id}).Count(&rootCount).Error)
			assert.EqualValues(t, 2, rootCount)
			replayed := adminUserRequest(http.MethodPost, "/api/user/batch/delete", body, proof, identity, common.RoleRootUser, nil, DeleteUserBatch)
			assert.Contains(t, replayed.Body.String(), "SECURITY_PROOF_CONSUMED")
			var audit model.AuditLog
			require.NoError(t, model.LOG_DB.Where("action = ?", "user.batch_delete").First(&audit).Error)
			assert.Equal(t, "Deleted 2 user(s); skipped 3.", audit.Content)
			payload, err := common.Marshal(audit)
			require.NoError(t, err)
			assert.NotContains(t, string(payload), proof)
			assert.NotContains(t, string(payload), key.Key)
		})
	}
}
