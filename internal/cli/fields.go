package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/parser"
)

// newRuleFieldsCmd returns the `nethelper rule fields` subcommand.
// When fr and reg are nil (normal CLI path) the package-level globals
// fieldRegistry and registry are used instead; tests pass non-nil values
// directly so they don't need a full database bootstrap.
func newRuleFieldsCmd(fr *parser.FieldRegistry, reg *parser.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "fields [vendor] [command]",
		Short: "Browse parser output fields",
		Long: `Browse the field catalog for parsed command outputs.

  nethelper rule fields                        # list all vendors
  nethelper rule fields huawei                 # list all CommandTypes for huawei
  nethelper rule fields huawei "display interface brief"  # list fields for that command`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve: prefer explicitly-injected instances (tests), fall back to globals.
			efr := fr
			if efr == nil {
				efr = fieldRegistry
			}
			ereg := reg
			if ereg == nil {
				ereg = registry
			}

			w := cmd.OutOrStdout()

			switch len(args) {
			case 0:
				vendors := efr.Vendors()
				if len(vendors) == 0 {
					fmt.Fprintln(w, "No vendors registered.")
					return nil
				}
				fmt.Fprintln(w, "Registered vendors:")
				for _, v := range vendors {
					fmt.Fprintf(w, "  %s\n", v)
				}

			case 1:
				vendor := args[0]
				types := efr.CmdTypes(vendor)
				if types == nil {
					return fmt.Errorf("unknown vendor %q", vendor)
				}
				fmt.Fprintf(w, "Vendor: %s\n", vendor)
				fmt.Fprintf(w, "%-40s  Fields\n", "CommandType")
				fmt.Fprintln(w, strings.Repeat("─", 60))
				for _, ct := range types {
					defs := efr.Fields(vendor, ct)
					fmt.Fprintf(w, "%-40s  %d\n", string(ct), len(defs))
				}

			default:
				vendor := args[0]
				rawCmd := strings.Join(args[1:], " ")

				p, ok := ereg.Get(vendor)
				if !ok {
					return fmt.Errorf("unknown vendor %q", vendor)
				}
				cmdType := p.ClassifyCommand(rawCmd)

				defs := efr.Fields(vendor, cmdType)
				if defs == nil {
					return fmt.Errorf("no fields registered for vendor=%q command=%q (CommandType=%q)", vendor, rawCmd, cmdType)
				}

				fmt.Fprintf(w, "Vendor: %s  Command: %s  (CommandType: %s)\n", vendor, rawCmd, cmdType)
				fmt.Fprintf(w, "%-20s  %-8s  %-8s  %-30s  %s\n", "Field", "Type", "Derived", "From", "Description")
				fmt.Fprintln(w, strings.Repeat("─", 80))
				for _, d := range defs {
					derived := "no"
					from := "—"
					if d.Derived {
						derived = "yes"
						from = strings.Join(d.DerivedFrom, ",")
					}
					fmt.Fprintf(w, "%-20s  %-8s  %-8s  %-30s  %s\n",
						d.Name, string(d.Type), derived, from, d.Description)
				}
			}
			return nil
		},
	}
}
