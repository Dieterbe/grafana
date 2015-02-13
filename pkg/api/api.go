package api

import (
	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/binding"
	"github.com/grafana/grafana/pkg/api/dtos"
	"github.com/grafana/grafana/pkg/middleware"
	m "github.com/grafana/grafana/pkg/models"
)

// Register adds http routes
func Register(r *macaron.Macaron) {
	reqSignedIn := middleware.Auth(&middleware.AuthOptions{ReqSignedIn: true})
	reqGrafanaAdmin := middleware.Auth(&middleware.AuthOptions{ReqSignedIn: true, ReqGrafanaAdmin: true})
	reqEditorRole := middleware.RoleAuth(m.ROLE_EDITOR, m.ROLE_ADMIN)
	reqAccountAdmin := middleware.RoleAuth(m.ROLE_ADMIN)
	bind := binding.Bind

	// not logged in views
	r.Get("/", reqSignedIn, Index)
	r.Get("/logout", Logout)
	r.Post("/login", bind(dtos.LoginCommand{}), LoginPost)
	r.Get("/login/:name", OAuthLogin)
	r.Get("/login", LoginView)

	// authed views
	r.Get("/profile/", reqSignedIn, Index)
	r.Get("/account/", reqSignedIn, Index)
	r.Get("/account/datasources/", reqSignedIn, Index)
	r.Get("/account/users/", reqSignedIn, Index)
	r.Get("/account/apikeys/", reqSignedIn, Index)
	r.Get("/account/import/", reqSignedIn, Index)
	r.Get("/admin/users", reqGrafanaAdmin, Index)
	r.Get("/dashboard/*", reqSignedIn, Index)
	r.Get("/location/", reqSignedIn, Index)
	r.Get("/monitor/", reqSignedIn, Index)
	r.Get("/site/", reqSignedIn, Index)
	// sign up
	r.Get("/signup", Index)
	r.Post("/api/user/signup", bind(m.CreateUserCommand{}), SignUp)

	// authed api
	r.Group("/api", func() {
		// user
		r.Group("/user", func() {
			r.Get("/", GetUser)
			r.Put("/", bind(m.UpdateUserCommand{}), UpdateUser)
			r.Post("/using/:id", SetUsingAccount)
			r.Get("/accounts", GetUserAccounts)
			r.Post("/stars/dashboard/:id", StarDashboard)
			r.Delete("/stars/dashboard/:id", UnstarDashboard)
		})

		// account
		r.Get("/account", GetAccount)
		r.Group("/account", func() {
			r.Post("/", bind(m.CreateAccountCommand{}), CreateAccount)
			r.Put("/", bind(m.UpdateAccountCommand{}), UpdateAccount)
			r.Post("/users", bind(m.AddAccountUserCommand{}), AddAccountUser)
			r.Get("/users", GetAccountUsers)
			r.Delete("/users/:id", RemoveAccountUser)
		}, reqAccountAdmin)

		// auth api keys
		r.Group("/auth/keys", func() {
			r.Combo("/").
				Get(GetApiKeys).
				Post(bind(m.AddApiKeyCommand{}), AddApiKey).
				Put(bind(m.UpdateApiKeyCommand{}), UpdateApiKey)
			r.Delete("/:id", DeleteApiKey)
		}, reqAccountAdmin)

		// Data sources
		r.Group("/datasources", func() {
			r.Combo("/").Get(GetDataSources).Put(AddDataSource).Post(UpdateDataSource)
			r.Delete("/:id", DeleteDataSource)
			r.Any("/proxy/:id/*", reqSignedIn, ProxyDataSourceRequest)
		}, reqAccountAdmin)

		// Dashboard
		r.Group("/dashboards", func() {
			r.Combo("/db/:slug").Get(GetDashboard).Delete(DeleteDashboard)
			r.Post("/db", reqEditorRole, bind(m.SaveDashboardCommand{}), PostDashboard)
			r.Get("/home", GetHomeDashboard)
		})

		// Search
		r.Get("/search/", Search)

		// metrics
		r.Get("/metrics/test", GetTestMetrics)

		// locations
		r.Group("/locations", func() {
			r.Combo("/").
				Get(bind(m.GetLocationsQuery{}), GetLocations).
				Put(AddLocation).
				Post(UpdateLocation)
			r.Get("/:id", GetLocationById)
			r.Delete("/:id", DeleteLocation)
		})

		// Monitors
		r.Group("/monitors", func() {
			r.Combo("/").
				Get(bind(m.GetMonitorsQuery{}), GetMonitors).
				Put(AddMonitor).Post(UpdateMonitor)
			r.Get("/:id", GetMonitorById)
			r.Delete("/:id", DeleteMonitor)
		})
		// sites
		r.Group("/sites", func() {
			r.Combo("/").Get(GetSites).Put(AddSite).Post(UpdateSite)
			r.Get("/:id", GetSiteById)
			r.Delete("/:id", DeleteSite)
		})
		r.Get("/monitor_types", GetMonitorTypes)

		r.Any("/graphite/*", GraphiteProxy)

	}, reqSignedIn)

	// admin api
	r.Group("/api/admin", func() {
		r.Get("/users", AdminSearchUsers)
	}, reqGrafanaAdmin)

	// rendering
	r.Get("/render/*", reqSignedIn, RenderToPng)

	r.NotFound(NotFound)
}