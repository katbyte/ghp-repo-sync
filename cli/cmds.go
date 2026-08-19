// Package cli implements the ghp-sync command line interface.
package cli

import (
	"errors"
	"fmt"

	"github.com/katbyte/ghp-sync/lib/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func ValidateParams(params []string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		for _, p := range params {
			if viper.GetString(p) == "" {
				return errors.New(p + " parameter can't be empty")
			}
		}

		return nil
	}
}

func Make(cmdName string) (*cobra.Command, error) {
	root := &cobra.Command{
		Use:           cmdName + " [command]",
		Short:         cmdName + " is a small utility to sync GitHub issues and PRs to a project",
		Long:          `Sync GitHub issues and PRs to a GitHub Project`,
		SilenceErrors: true,
		PreRunE:       ValidateParams([]string{"token", "repos", "project-owner", "project-number"}),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("USAGE: ghp-syc [issues|prs] katbyte/ghp-sync project")

			return nil
		},
	}

	root.AddCommand(&cobra.Command{
		Use:           "version",
		Short:         "Print the version",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(cmdName + " " + version.Version)
		},
	})

	root.AddCommand(&cobra.Command{
		Use:           "issues",
		Short:         "Sync issues from a repo to a project",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		PreRunE:       ValidateParams([]string{"token", "repos", "project-owner", "project-number"}),
		RunE:          CmdIssues,
	})

	prsCmd := &cobra.Command{
		Use:           "prs",
		Short:         "Sync PRs from a repo to a project",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		PreRunE:       ValidateParams([]string{"token", "repos", "project-owner", "project-number"}),
		RunE:          CmdPRs,
	}
	prsCmd.AddCommand(&cobra.Command{
		Use:   "refresh",
		Short: "Refresh fields for closed/merged PRs already on the project board",
		Long: `Walks the project board and updates fields on PR items that are now closed or merged,
without crawling the repo's full PR history. Use -r to limit to specific repos, --pr-populate-fields
to override the default field list, and --dry-run to preview.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		PreRunE:       ValidateParams([]string{"token", "project-owner", "project-number"}),
		RunE:          CmdPRsRefresh,
	})
	root.AddCommand(prsCmd)

	addCmd := &cobra.Command{
		Use:   "add [field[:type] ...]",
		Short: "Add PRs/issues to a project from CSV on stdin (url,col1,col2,...), mapping each column to the named project field",
		Long: `Add PRs/issues to a project from CSV on stdin, where the first column is a github PR/issue URL
and the remaining columns map to project fields. With no args the first line is a header naming
the field for each column (the url column's header is ignored); positional args name the fields
instead for headerless input. Use '-' to skip a column, and an optional ':text', ':number',
':date' suffix to force a field type (single select fields are detected automatically and
mapped by option name).

  rjg list | ghp-sync add -o katbyte -p 42 --set "Status=Backlog [PRs]"
  rjg list | ghp-sync add "JIRA" "JIRA URL" -o katbyte -p 42   # headerless input`,
		SilenceErrors: true,
		PreRunE:       ValidateParams([]string{"token", "project-owner", "project-number"}),
		RunE:          CmdAdd,
	}
	addCmd.Flags().StringSlice("set", []string{}, "set a fixed field value on every added item, e.g. 'Status=Backlog [PRs]' (repeatable)")
	root.AddCommand(addCmd)

	root.AddCommand(&cobra.Command{
		Use:           "project source-project-owner source-project-number",
		Short:         "Sync issues and PRs between two projects",
		Args:          cobra.ExactArgs(2),
		SilenceErrors: true,
		PreRunE:       ValidateParams([]string{"token", "project-owner", "project-number"}),
		RunE:          CmdSync,
	})

	// command to get and print gh rate limits
	root.AddCommand(&cobra.Command{
		Use:           "rate-limits",
		Short:         "get and print github api rate limits",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		PreRunE:       ValidateParams([]string{"token"}),
		RunE:          CmdRateLimit,
	})

	if err := configureFlags(root); err != nil {
		return nil, fmt.Errorf("unable to configure flags: %w", err)
	}

	return root, nil
}
