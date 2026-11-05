package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/UnlockedLabs/unlocked-contracts/coreapi"
	"github.com/UnlockedLabs/unlocked-contracts/identity"
	"github.com/UnlockedLabs/unlocked-contracts/users"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/database"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/module"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/providerplatforms"
	"github.com/UnlockedLabs/unlocked-provider-platforms/internal/server"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These tests pin what every Provider Platforms route answers (EN-193): the
// status and the JSON. They are core's scenarios, run against this service's
// handlers behind the service token and X-Identity-Claims header. Who may call
// a route is core's check now, so core's access matrix and its CSRF cases
// are not repeated here.

const serviceToken = "integration-token"

type testFacility struct {
	ID   uint
	Name string
}

type testUser struct {
	ID         uint
	Username   string
	Role       users.Role
	FacilityID uint
}

type providerPlatformsWorld struct {
	north, south *testFacility
	// jdoe is mapped to the Kolibri provider and has its learning data.
	// mmajor matches a provider user by name. newbie gets mapped by a test.
	resident, mmajor, newbie, southResident *testUser
	facAdmin, deptAdmin                     *testUser
}

func (w providerPlatformsWorld) claims(u *testUser) *identity.Claims {
	return &identity.Claims{UserID: u.ID, Username: u.Username, Role: u.Role, FacilityID: u.FacilityID, FacilityName: w.north.Name}
}

func createFacility(t *testing.T, db *gorm.DB, name string) *testFacility {
	t.Helper()
	f := &testFacility{Name: name}
	require.NoError(t, db.Raw(`INSERT INTO facilities (name, timezone, created_at, updated_at) VALUES (?, 'America/Chicago', now(), now()) RETURNING id`, name).Scan(&f.ID).Error)
	return f
}

func createUser(t *testing.T, db *gorm.DB, username string, role users.Role, facilityID uint, docID string) *testUser {
	t.Helper()
	u := &testUser{Username: username, Role: role, FacilityID: facilityID}
	var doc *string
	if docID != "" {
		doc = &docID
	}
	require.NoError(t, db.Raw(`INSERT INTO users (username, name_first, name_last, role, email, doc_id, facility_id, created_at, updated_at)
		VALUES (?, 'Test', 'User', ?, ?, ?, ?, now(), now()) RETURNING id`,
		username, role, username+"@example.com", doc, facilityID).Scan(&u.ID).Error)
	return u
}

// coreUsers stands in for core's Users: it adds and looks up rows in core's
// users table, and gets a login's Kratos id from the fake Kratos.
type coreUsers struct {
	db   *gorm.DB
	fake *fakeExternal
}

func (c coreUsers) Get(_ context.Context, id uint) (coreapi.User, error) {
	var u coreapi.User
	res := c.db.Raw(`SELECT id, username, name_first, name_last, email, facility_id FROM users WHERE id = ? AND deleted_at IS NULL`, id).Scan(&u)
	if res.Error != nil {
		return u, res.Error
	}
	if res.RowsAffected == 0 {
		return u, fmt.Errorf("user %d: %w", id, coreapi.ErrUserNotFound)
	}
	return u, nil
}

func (c coreUsers) Create(ctx context.Context, nu coreapi.NewUser) (coreapi.User, error) {
	var creator *uint
	if cl, ok := identity.FromContext(ctx); ok {
		creator = &cl.UserID
	}
	// Core's users table is unique on username where its tests run on SQLite.
	var taken int64
	if err := c.db.Raw(`SELECT count(*) FROM users WHERE username = ?`, nu.Username).Scan(&taken).Error; err != nil {
		return coreapi.User{}, err
	}
	if taken > 0 {
		return coreapi.User{}, fmt.Errorf("create user %s: duplicate username", nu.Username)
	}
	u := coreapi.User{Username: nu.Username, NameFirst: nu.NameFirst, NameLast: nu.NameLast, Email: nu.Email, FacilityID: nu.FacilityID}
	err := c.db.Raw(`INSERT INTO users (username, name_first, name_last, email, facility_id, role, create_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, now(), now()) RETURNING id`,
		nu.Username, nu.NameFirst, nu.NameLast, nu.Email, nu.FacilityID, nu.Role, creator).Scan(&u.ID).Error
	return u, err
}

func (c coreUsers) CreateLogin(_ context.Context, userID uint) (string, error) {
	resp, err := http.Post(c.fake.URL+"/admin/identities", "application/json", strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var identity struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return "", err
	}
	if err := c.db.Exec(`UPDATE users SET kratos_id = ? WHERE id = ?`, identity.ID, userID).Error; err != nil {
		return "", err
	}
	return "temp-password", nil
}

func (c coreUsers) Delete(_ context.Context, id uint) error {
	return c.db.Exec(`UPDATE users SET deleted_at = now() WHERE id = ?`, id).Error
}

// noLiveProviders is a core with no live Canvas or Essential Education
// providers: every provider's users come from the worker.
type noLiveProviders struct{}

func (noLiveProviders) Users(context.Context, uint) ([]coreapi.ProviderUser, bool, error) {
	return nil, false, nil
}

func (noLiveProviders) MappingsChanged(uint, uint) {}

type routeList []module.Route

func (r *routeList) Handle(rt module.Route) { *r = append(*r, rt) }

// adminDB is a connection as the database owner, for seeding core's tables.
func adminDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(database.DSN() + " search_path=provider,public"))
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

// serviceClient calls the service's handlers over HTTP, with the service
// token and the caller's claims, and does not follow redirects.
type serviceClient struct {
	url  string
	http *http.Client
}

func newService(t *testing.T, db *gorm.DB, fake *fakeExternal) *serviceClient {
	t.Helper()
	pdb, sqlDB, err := database.OpenAs(providerplatforms.Module{}.Schema())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	var routes routeList
	providerplatforms.Module{}.Register(&routes, module.Deps{DB: pdb, Users: coreUsers{db: db, fake: fake}, LiveProviders: noLiveProviders{}})
	h, err := server.NewHandler("", server.Version{}, server.Routes(serviceToken, routes))
	require.NoError(t, err)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return &serviceClient{url: ts.URL, http: &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// do sends the request and returns the status and raw body.
func (c *serviceClient) do(t *testing.T, claims *identity.Claims, method, path string, body any) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.url+path, rdr)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+serviceToken)
	require.NoError(t, identity.SetClaimsHeader(req, claims))
	resp, err := c.http.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(raw)
}

// seedProviderPlatforms adds three providers, an OIDC client, two courses,
// and jdoe's mapping, enrollment, milestone, outcomes and activity. The
// Kolibri provider points at the fake and its access key is stored encrypted
// with APP_KEY "test-app-key", the way the API stores it.
func seedProviderPlatforms(t *testing.T, db *gorm.DB, fake *fakeExternal) providerPlatformsWorld {
	t.Helper()
	t.Setenv("APP_KEY", "test-app-key")
	t.Setenv("APP_URL", "https://unlocked.test")
	t.Setenv("PROVIDER_SERVICE_URL", fake.URL)
	t.Setenv("KRATOS_ADMIN_URL", fake.URL)
	t.Setenv("HYDRA_ADMIN_URL", fake.URL)

	// Core seeds a default facility and admin first, so the test users and
	// facilities get the IDs the golden files have.
	createFacility(t, db, "Default")
	createUser(t, db, "SuperAdmin", users.SystemAdmin, 1, "")
	var w providerPlatformsWorld
	w.north = createFacility(t, db, "North")
	w.south = createFacility(t, db, "South")
	for _, u := range []struct {
		user     **testUser
		username string
		role     users.Role
		facility uint
		docID    string
	}{
		{&w.resident, "jdoe", users.Student, w.north.ID, "1001"},
		{&w.mmajor, "mmajor", users.Student, w.north.ID, "1002"},
		{&w.newbie, "newbie", users.Student, w.north.ID, "1003"},
		{&w.southResident, "southres", users.Student, w.south.ID, "1004"},
		{&w.facAdmin, "facadmin", users.FacilityAdmin, w.north.ID, ""},
		{&w.deptAdmin, "deptadmin", users.DepartmentAdmin, w.north.ID, ""},
	} {
		*u.user = createUser(t, db, u.username, u.role, u.facility, u.docID)
	}

	day := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	const kolibriKey = "MDEyMzQ1Njc4OWFiY2RlZsFxCXqsaoGNjd91872zRlhh9w==" // "admin:kolibri-pass"
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE users SET name_first = 'John', name_last = 'Doe' WHERE id = ?`, []any{w.resident.ID}},
		{`UPDATE users SET name_first = 'Mary', name_last = 'Major' WHERE id = ?`, []any{w.mmajor.ID}},
		{`INSERT INTO provider_platforms (id, type, name, account_id, access_key, base_url, state, external_auth_provider_id, created_at, updated_at) VALUES
			(1, 'kolibri', 'Kolibri', 'kolibri-facility', ?, ?, 'enabled', '', ?, ?),
			(2, 'brightspace', 'Brightspace', 'bs-account', 'client-id;refresh-token', 'https://brightspace.example', 'enabled', 'bs-auth', ?, ?),
			(3, 'canvas_cloud', 'Old Canvas', 'canvas-account', 'canvas-token', 'https://canvas.example', 'archived', '', ?, ?)`,
			[]any{kolibriKey, fake.URL, day, day, day, day, day, day}},
		{`INSERT INTO oidc_clients (id, provider_platform_id, client_id, client_name, client_secret, redirect_uris, scopes, created_at, updated_at)
			VALUES (1, 2, 'bs-client', 'Brightspace', 'bs-secret', 'https://brightspace.example/callback', 'openid offline profile email', ?, ?)`, []any{day, day}},
		{`INSERT INTO provider_user_mappings (user_id, provider_platform_id, external_user_id, external_username, authentication_provider_status, created_at, updated_at)
			VALUES (?, 1, 'k-1001', 'john.doe', 'none', ?, ?)`, []any{w.resident.ID, day, day}},
		{`INSERT INTO courses (id, provider_platform_id, name, description, external_id, thumbnail_url, type, outcome_types, external_url, total_progress_milestones, created_at, updated_at) VALUES
			(1, 1, 'Algebra Basics', 'Numbers and equations', 'k-course-1', '/thumbs/algebra.png', 'fixed_enrollment', 'grade,certificate', ?, 4, ?, ?),
			(2, 1, 'Biology', NULL, 'k-course-2', NULL, 'open_enrollment', NULL, NULL, 2, ?, ?)`,
			[]any{fake.URL + "/learn/algebra", day, day, day.Add(time.Hour), day.Add(time.Hour)}},
		{`INSERT INTO user_enrollments (user_id, course_id, external_id, created_at, updated_at) VALUES (?, 1, 'k-enroll-1', ?, ?)`,
			[]any{w.resident.ID, day, day}},
		{`INSERT INTO milestones (user_id, course_id, external_id, type, is_completed, created_at, updated_at) VALUES
			(?, 1, 'k-milestone-1', 'quiz_submission', true, ?, ?)`, []any{w.resident.ID, day, day}},
		{`INSERT INTO outcomes (type, course_id, user_id, value, created_at, updated_at) VALUES
			('grade', 1, ?, 'A', ?, ?), ('certificate', 1, ?, 'cert-1', ?, ?)`,
			[]any{w.resident.ID, day, day, w.resident.ID, day, day}},
		{`INSERT INTO activities (course_id, user_id, type, total_time, time_delta, external_id, created_at, updated_at) VALUES
			(1, ?, 'interaction', 600, 600, 'k-activity-1', ?, ?),
			(1, ?, 'completion', 900, 300, 'k-activity-2', ?, ?)`,
			[]any{w.resident.ID, day, day, w.resident.ID, day.Add(24 * time.Hour), day.Add(24 * time.Hour)}},
		{`INSERT INTO user_course_activity_totals (user_id, course_id, total_time, last_ts) VALUES (?, 1, 900, ?)`,
			[]any{w.resident.ID, day.Add(24 * time.Hour)}},
	} {
		require.NoError(t, db.Exec(stmt.sql, stmt.args...).Error, stmt.sql)
	}
	// The inserts above set their own IDs, so move each sequence past them.
	for _, table := range []string{"provider_platforms", "oidc_clients", "courses"} {
		require.NoError(t, db.Exec(fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%[1]s', 'id'), (SELECT max(id) FROM %[1]s))`, table)).Error)
	}
	return w
}

func TestProviderPlatformRoutes(t *testing.T) {
	if skipReason != "" {
		t.Skip(skipReason)
	}
	db := adminDB(t)
	fake := newFakeExternal(t)
	w := seedProviderPlatforms(t, db, fake)
	svc := newService(t, db, fake)

	resident, facAdmin := w.claims(w.resident), w.claims(w.facAdmin)
	user := func(format string, u *testUser) string { return fmt.Sprintf(format, u.ID) }
	activityRange := "start_date=2026-01-01&end_date=2026-01-10"

	// The cases run in order against one database, so later reads show the
	// earlier writes. mask lists JSON fields that change every run.
	tests := []struct {
		name   string
		claims *identity.Claims
		method string
		path   string
		body   any
		status int
		mask   []string
	}{
		// Provider platforms
		{"platforms", facAdmin, http.MethodGet, "/api/provider-platforms", nil, http.StatusOK, nil},
		{"platforms_oidc_enabled", facAdmin, http.MethodGet, "/api/provider-platforms?only=oidc_enabled", nil, http.StatusOK, nil},
		{"platform_by_id", facAdmin, http.MethodGet, "/api/provider-platforms/1", nil, http.StatusOK, nil},
		{"platform_by_id_bad_id", facAdmin, http.MethodGet, "/api/provider-platforms/abc", nil, http.StatusBadRequest, nil},
		{"platform_by_id_missing", facAdmin, http.MethodGet, "/api/provider-platforms/999", nil, http.StatusOK, nil},
		{"platform_create", facAdmin, http.MethodPost, "/api/provider-platforms",
			map[string]any{"type": "canvas_cloud", "name": "New Canvas", "account_id": "1", "access_key": "new-token", "base_url": " https://new-canvas.example ", "state": "disabled"},
			http.StatusCreated, []string{"access_key"}},
		{"platform_create_http", facAdmin, http.MethodPost, "/api/provider-platforms",
			map[string]any{"type": "canvas_cloud", "name": "Plain", "base_url": "http://plain.example"}, http.StatusBadRequest, nil},
		{"platform_create_brightspace", facAdmin, http.MethodPost, "/api/provider-platforms",
			map[string]any{"type": "brightspace", "name": "BS", "account_id": "id", "access_key": "secret", "base_url": "https://bs.example"},
			http.StatusInternalServerError, nil},
		{"platform_update", facAdmin, http.MethodPatch, "/api/provider-platforms/3", map[string]any{"name": "Older Canvas"}, http.StatusOK, nil},
		{"platform_update_http", facAdmin, http.MethodPatch, "/api/provider-platforms/3", map[string]any{"base_url": "http://plain.example"}, http.StatusBadRequest, nil},
		// Brightspace needs the OAuth state bucket, which the tests don't run.
		{"platform_refresh", facAdmin, http.MethodGet, "/api/provider-platforms/2/refresh", nil, http.StatusInternalServerError, nil},
		{"platform_callback_no_state", facAdmin, http.MethodGet, "/api/provider-platforms/callback", nil, http.StatusTemporaryRedirect, nil},
		{"platform_delete", facAdmin, http.MethodDelete, "/api/provider-platforms/4", nil, http.StatusNoContent, nil},
		{"platforms_after_changes", facAdmin, http.MethodGet, "/api/provider-platforms", nil, http.StatusOK, nil},

		// Logins and mappings
		{"logins", facAdmin, http.MethodGet, user("/api/users/%d/logins", w.resident), nil, http.StatusOK, nil},
		{"login_create", facAdmin, http.MethodPost, user("/api/users/%d/logins", w.southResident),
			map[string]any{"user_id": w.southResident.ID, "provider_platform_id": 1, "external_user_id": "k-3001", "external_username": "south.res"},
			http.StatusCreated, nil},
		{"logins_after_create", facAdmin, http.MethodGet, user("/api/users/%d/logins", w.southResident), nil, http.StatusOK, nil},
		{"login_delete", facAdmin, http.MethodDelete, user("/api/users/%d/logins/1", w.southResident), nil, http.StatusNoContent, nil},
		{"logins_after_delete", facAdmin, http.MethodGet, user("/api/users/%d/logins", w.southResident), nil, http.StatusOK, nil},
		{"mapped_users", facAdmin, http.MethodGet, "/api/provider-platforms/1/mapped-users", nil, http.StatusOK, nil},
		{"map_user", facAdmin, http.MethodPost, user("/api/provider-platforms/1/map-user/%d", w.newbie),
			map[string]any{"external_user_id": "k-4001", "external_username": "new.bie"}, http.StatusCreated, nil},
		{"map_user_bad_body", facAdmin, http.MethodPost, user("/api/provider-platforms/1/map-user/%d", w.newbie), "not an object", http.StatusBadRequest, nil},
		{"create_user_account", facAdmin, http.MethodPost, user("/api/provider-platforms/1/create-user/%d", w.southResident), nil, http.StatusCreated, nil},
		{"user_account", facAdmin, http.MethodPost, user("/api/provider-platforms/1/user-accounts/%d", w.southResident), nil, http.StatusCreated, nil},
		{"users_import", facAdmin, http.MethodPost, "/api/provider-platforms/1/users/import",
			map[string]any{"users": []providerUser{
				{Username: "pat.lee", NameFirst: "Pat", NameLast: "Lee", Email: "pat@example.com", ExternalUserID: "k-5001", ExternalUsername: "pat.lee"},
				{Username: "jdoe", NameFirst: "Duplicate", NameLast: "Name", ExternalUserID: "k-5002"},
			}}, http.StatusOK, []string{"temp_password"}},
		{"users_import_bad_body", facAdmin, http.MethodPost, "/api/provider-platforms/1/users/import", "not an object", http.StatusBadRequest, nil},

		// Actions: the worker's provider users and matching them to residents
		{"action_get_users", facAdmin, http.MethodGet, "/api/actions/provider-platforms/1/get-users", nil, http.StatusOK, nil},
		{"action_get_users_search", facAdmin, http.MethodGet, "/api/actions/provider-platforms/1/get-users?search=Jane", nil, http.StatusOK, nil},
		{"action_get_users_bad_id", facAdmin, http.MethodGet, "/api/actions/provider-platforms/abc/get-users", nil, http.StatusBadRequest, nil},
		{"action_match_users", facAdmin, http.MethodGet, "/api/actions/provider-platforms/1/match-users", nil, http.StatusOK, nil},
		{"action_apply_matches", facAdmin, http.MethodPost, "/api/actions/provider-platforms/1/apply-matches",
			map[string]any{
				"confirmed": []map[string]any{{"canvas_user": unmappedProviderUsers[0], "unlocked_user_id": w.mmajor.ID}},
				"to_create": []providerUser{unmappedProviderUsers[1]},
			}, http.StatusOK, nil},
		{"action_apply_matches_other_facility", facAdmin, http.MethodPost, "/api/actions/provider-platforms/1/apply-matches",
			map[string]any{"confirmed": []map[string]any{{"canvas_user": unmappedProviderUsers[0], "unlocked_user_id": w.southResident.ID}}},
			http.StatusOK, nil},
		{"action_import_users", facAdmin, http.MethodPost, "/api/actions/provider-platforms/1/import-users", nil, http.StatusOK, nil},
		{"mapped_users_after_actions", facAdmin, http.MethodGet, "/api/provider-platforms/1/mapped-users?per_page=50", nil, http.StatusOK, nil},

		// OIDC clients
		{"oidc_clients", facAdmin, http.MethodGet, "/api/oidc/clients", nil, http.StatusOK, nil},
		{"oidc_client", facAdmin, http.MethodGet, "/api/oidc/clients/1", nil, http.StatusOK, nil},
		{"oidc_register", facAdmin, http.MethodPost, "/api/oidc/clients", map[string]any{"provider_platform_id": 1}, http.StatusCreated, nil},
		{"oidc_register_again", facAdmin, http.MethodPost, "/api/oidc/clients", map[string]any{"provider_platform_id": 1}, http.StatusBadRequest, nil},
		{"oidc_recreate", facAdmin, http.MethodPost, "/api/oidc/clients/1/recreate", nil, http.StatusCreated, nil},
		{"oidc_recreate_bad_id", facAdmin, http.MethodPost, "/api/oidc/clients/abc/recreate", nil, http.StatusBadRequest, nil},
		{"oidc_clients_after_changes", facAdmin, http.MethodGet, "/api/oidc/clients", nil, http.StatusOK, nil},
		{"platforms_oidc_enabled_after_changes", facAdmin, http.MethodGet, "/api/provider-platforms?only=oidc_enabled", nil, http.StatusOK, nil},

		// Courses, outcomes and activity
		{"courses", facAdmin, http.MethodGet, "/api/courses", nil, http.StatusOK, nil},
		{"courses_search", facAdmin, http.MethodGet, "/api/courses?search=bio", nil, http.StatusOK, nil},
		{"course_by_id", resident, http.MethodGet, "/api/courses/1", nil, http.StatusOK, nil},
		{"course_by_id_bad_id", resident, http.MethodGet, "/api/courses/abc", nil, http.StatusBadRequest, nil},
		{"course_activity", facAdmin, http.MethodGet, "/api/courses/1/activity", nil, http.StatusOK, nil},
		{"outcomes", resident, http.MethodGet, user("/api/users/%d/outcomes", w.resident), nil, http.StatusOK, nil},
		{"outcomes_by_type", resident, http.MethodGet, user("/api/users/%d/outcomes?type=certificate", w.resident), nil, http.StatusOK, nil},
		{"daily_activity", resident, http.MethodGet, user("/api/users/%d/daily-activity?", w.resident) + activityRange, nil, http.StatusOK, nil},
		{"daily_activity_bad_date", resident, http.MethodGet, user("/api/users/%d/daily-activity?start_date=soon", w.resident), nil, http.StatusBadRequest, nil},
		{"catalog", resident, http.MethodGet, user("/api/users/%d/catalog", w.resident), nil, http.StatusOK, nil},
		{"catalog_search", resident, http.MethodGet, user("/api/users/%d/catalog?search=bio", w.resident), nil, http.StatusOK, nil},
		{"user_courses", resident, http.MethodGet, user("/api/users/%d/courses", w.resident), nil, http.StatusOK, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, raw := svc.do(t, tt.claims, tt.method, tt.path, tt.body)
			require.Equal(t, tt.status, status, raw)
			body := strings.ReplaceAll(raw, fake.URL, "http://fake.test")
			expectGolden(t, "provider_platforms", tt.name, maskFields(t, body, tt.mask...))
		})
	}

	// platform_create masks the access key because it's encrypted with a
	// random IV, so check here that it isn't stored as sent.
	var stored string
	require.NoError(t, db.Raw(`SELECT access_key FROM provider_platforms WHERE name = 'New Canvas'`).Scan(&stored).Error)
	require.NotEmpty(t, stored)
	require.NotEqual(t, "new-token", stored)
}
