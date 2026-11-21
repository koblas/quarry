package cli

import (
	"github.com/spf13/cobra"
)

// newRootCommand builds quarry's command tree: a persistent --json flag and a
// root-only --version flag on the root plus the sync, status, accounts,
// holdings, acb, networth, spend, cashflow, recurring, anomalies, summary,
// search, findings, sql, snapshots (with its prune child), mcp, claude (with
// its install and uninstall children) and version subcommands, wired against
// env's factories and build info.
func newRootCommand(env Env, jsonOut *bool) *cobra.Command {
	var showVersion bool
	// Args stays nil: cobra's legacy check then refuses an unmatched subcommand
	// instead of letting the RunE below accept it as a positional argument.
	root := &cobra.Command{
		Use:   "quarry",
		Short: "Snapshot and query Quicken Classic for Mac data locally",
		Long: `quarry copies the Quicken Classic for Mac file you have open into a local,
read-only snapshot, rebuilds its own store from that snapshot, and checks the
store against Quicken's balances. Every other command reads that store;
quarry never writes to the Quicken file.`,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableSuggestions: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				return printVersion(cmd, env.BuildInfo, *jsonOut)
			}
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().BoolVar(jsonOut, "json", false, "print the result as JSON on stdout")
	root.Flags().BoolVar(&showVersion, "version", false, "print quarry's version")

	root.AddCommand(newSyncCommand(env.NewServer, env.LoadConfig, jsonOut))
	root.AddCommand(newStatusCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newAccountsCommand(env.NewReport, env.LoadConfig, jsonOut))
	root.AddCommand(newSpendCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newHoldingsCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newAcbCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newNetWorthCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newCashFlowCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newRecurringCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newAnomaliesCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newSummaryCommand(env.NewReport, env.LoadConfig, env.Now, jsonOut))
	root.AddCommand(newSearchCommand(env.NewReport, jsonOut))
	root.AddCommand(newFindingsCommand(env.NewReport, env.LoadConfig, jsonOut))
	root.AddCommand(newSQLCommand(env.NewReport, jsonOut))
	root.AddCommand(newSnapshotsCommand(env.NewSnapshots, env.LoadConfig, jsonOut))
	root.AddCommand(newMCPCommand(env.ServeMCP, env.IsTerminal, jsonOut))
	root.AddCommand(newClaudeCommand(env.RunTool, env.LookPath, env.Home, env.Executable, jsonOut))
	root.AddCommand(newVersionCommand(env.BuildInfo, jsonOut))
	return root
}
