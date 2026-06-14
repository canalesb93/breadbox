//go:build !headless && !lite

package admin

import (
	"context"
	"fmt"

	"breadbox/internal/app"
	"breadbox/internal/appconfig"
	"breadbox/internal/templates/components"
)

// OnboardingProgress is the shared, DB-derived view of how far setup has
// gotten. Both the dedicated /getting-started walkthrough and the home-feed
// "Finish setting up" banner read from this single source so the two never
// disagree about which step is next.
//
// Step order is fixed (1 provider, 2 household member [optional], 3
// connection, 4 sync, 5 agents); ActiveStep is the 1-based index of the first
// incomplete step (0 when AllComplete).
type OnboardingProgress struct {
	HasProvider   bool
	HasMember     bool
	HasConnection bool
	HasSync       bool
	HasAPIKey     bool

	CompletedSteps int
	TotalSteps     int
	ActiveStep     int
	AllComplete    bool
	TimeRemaining  string

	Dismissed bool

	MemberCount      int64
	ConnectionCount  int64
	AccountCount     int64
	TransactionCount int64
	SuccessfulSyncs  int64
	ApiKeyCount      int64
}

// computeOnboardingProgress runs the step-completion checks against the live
// database. Query errors are logged and treated as "not done" so a transient
// failure never falsely marks setup complete.
func computeOnboardingProgress(ctx context.Context, a *app.App) OnboardingProgress {
	logErr := func(what string, err error) {
		if err != nil {
			a.Logger.Error("onboarding progress: "+what, "error", err)
		}
	}

	// Step 1: a bank provider is configured (Plaid, Teller, or SimpleFIN).
	hasProvider := a.Config.PlaidClientID != "" || a.Config.TellerAppID != "" || a.Config.SimpleFINEnabled

	// Step 2 (optional): household has a member. Admin creation seeds one, so
	// this is effectively auto-complete.
	userCount, err := a.Queries.CountUsers(ctx)
	logErr("count users", err)

	// Step 3: at least one bank connection.
	connCount, err := a.Queries.CountConnections(ctx)
	logErr("count connections", err)

	// Step 4: first successful sync.
	successfulSyncs, err := a.Queries.CountSuccessfulSyncs(ctx)
	logErr("count successful syncs", err)

	// Step 5: at least one active API key (agents).
	activeAPIKeys, err := a.Queries.CountActiveApiKeys(ctx)
	logErr("count active api keys", err)

	accountCount, err := a.Queries.CountAccounts(ctx)
	logErr("count accounts", err)

	txCount, err := a.Queries.CountTransactions(ctx)
	logErr("count transactions", err)

	// Resolve active step + a rough time-to-finish estimate. Per-step minutes
	// are paired with the step order so the estimate sums only outstanding
	// steps.
	stepsDone := []bool{hasProvider, userCount > 0, connCount > 0, successfulSyncs > 0, activeAPIKeys > 0}
	stepMinutes := []int{2, 1, 2, 1, 3}
	completed := 0
	activeStep := 0
	minutesLeft := 0
	for i, done := range stepsDone {
		if done {
			completed++
			continue
		}
		if activeStep == 0 {
			activeStep = i + 1 // 1-based
		}
		minutesLeft += stepMinutes[i]
	}
	allComplete := completed == len(stepsDone)
	timeRemaining := ""
	if !allComplete && minutesLeft > 0 {
		timeRemaining = fmt.Sprintf("~%d min left", minutesLeft)
	}

	return OnboardingProgress{
		HasProvider:      hasProvider,
		HasMember:        userCount > 0,
		HasConnection:    connCount > 0,
		HasSync:          successfulSyncs > 0,
		HasAPIKey:        activeAPIKeys > 0,
		CompletedSteps:   completed,
		TotalSteps:       len(stepsDone),
		ActiveStep:       activeStep,
		AllComplete:      allComplete,
		TimeRemaining:    timeRemaining,
		Dismissed:        appconfig.Bool(ctx, a.Queries, "onboarding_dismissed", false),
		MemberCount:      userCount,
		ConnectionCount:  connCount,
		AccountCount:     accountCount,
		TransactionCount: txCount,
		SuccessfulSyncs:  successfulSyncs,
		ApiKeyCount:      activeAPIKeys,
	}
}

// onboardingStepDef is the static definition of one onboarding step — its
// label and the direct action the banner links to. CTA targets mirror the
// /getting-started walkthrough so the banner stays a faithful, self-sufficient
// stand-in once that page is retired.
type onboardingStepDef struct {
	title    string
	desc     string
	ctaLabel string
	ctaHref  string
	optional bool
}

// onboardingStepDefs is the fixed step order (matches OnboardingProgress's
// 1-based ActiveStep and the per-step done flags resolved below).
var onboardingStepDefs = []onboardingStepDef{
	{title: "Configure a bank provider", desc: "Set up Plaid, Teller, or SimpleFIN to link accounts — or import a CSV.", ctaLabel: "Configure", ctaHref: "/settings/providers"},
	{title: "Add a household member", desc: "Add the people whose accounts you track. Optional unless you share finances.", ctaLabel: "Add member", ctaHref: "/household/new", optional: true},
	{title: "Connect your bank", desc: "Link a bank account to start syncing accounts and transactions.", ctaLabel: "Connect bank", ctaHref: "/connections/new"},
	{title: "Run your first sync", desc: "Your first sync runs automatically — check the logs if it doesn't start.", ctaLabel: "View logs", ctaHref: "/logs"},
	{title: "Set up AI agents", desc: "Create an API key so AI agents can query your finances via MCP.", ctaLabel: "Set up", ctaHref: "/workflows"},
}

// onboardingBannerProps builds the home-feed "Finish setting up" checklist
// card from the progress, or returns nil when the banner should not show —
// setup is complete or the guide was dismissed. Keeping the show/hide decision
// here means callers just assign the result to FeedProps.Onboarding.
func onboardingBannerProps(p OnboardingProgress, csrfToken string) *components.OnboardingBannerProps {
	if p.AllComplete || p.Dismissed {
		return nil
	}
	done := []bool{p.HasProvider, p.HasMember, p.HasConnection, p.HasSync, p.HasAPIKey}
	steps := make([]components.OnboardingStep, 0, len(onboardingStepDefs))
	for i, def := range onboardingStepDefs {
		steps = append(steps, components.OnboardingStep{
			Title:    def.title,
			Desc:     def.desc,
			CTALabel: def.ctaLabel,
			CTAHref:  def.ctaHref,
			Done:     done[i],
			Active:   p.ActiveStep == i+1, // ActiveStep is 1-based
			Optional: def.optional,
		})
	}
	return &components.OnboardingBannerProps{
		CompletedSteps: p.CompletedSteps,
		TotalSteps:     p.TotalSteps,
		TimeRemaining:  p.TimeRemaining,
		Steps:          steps,
		CSRFToken:      csrfToken,
	}
}
