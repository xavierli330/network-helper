package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/graph"
)

func newShowCmd() *cobra.Command {
	show := &cobra.Command{
		Use:   "show",
		Short: "Query network data",
	}
	show.AddCommand(newShowDeviceCmd())
	show.AddCommand(newShowInterfaceCmd())
	show.AddCommand(newShowRouteCmd())
	show.AddCommand(newShowFIBCmd())
	show.AddCommand(newShowLabelCmd())
	show.AddCommand(newShowNeighborCmd())
	show.AddCommand(newShowTunnelCmd())
	show.AddCommand(newShowTopologyCmd())
	return show
}

func newShowTopologyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "topology",
		Short: "Show network topology overview",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := graph.BuildFromDB(db)
			if err != nil {
				return fmt.Errorf("build graph: %w", err)
			}

			fmt.Printf("Network Topology:\n")
			fmt.Printf("  Devices:    %d\n", len(g.NodesByType(graph.NodeTypeDevice)))
			fmt.Printf("  Interfaces: %d\n", len(g.NodesByType(graph.NodeTypeInterface)))
			fmt.Printf("  Subnets:    %d\n", len(g.NodesByType(graph.NodeTypeSubnet)))
			fmt.Printf("  Total nodes: %d\n", g.NodeCount())
			fmt.Printf("  Total edges: %d\n\n", g.EdgeCount())

			// List devices with their peer count
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "DEVICE\tHOSTNAME\tPEERS\tINTERFACES\n")
			for _, dev := range g.NodesByType(graph.NodeTypeDevice) {
				peers := g.NeighborsByType(dev.ID, graph.EdgePeer)
				ifaces := g.NeighborsByType(dev.ID, graph.EdgeHasInterface)
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", dev.ID, dev.Props["hostname"], len(peers), len(ifaces))
			}
			return w.Flush()
		},
	}
}

func newShowDeviceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "device [device-id]",
		Short: "Show device information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				d, err := db.GetDevice(args[0])
				if err != nil {
					return err
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintf(w, "ID:\t%s\n", d.ID)
				fmt.Fprintf(w, "Hostname:\t%s\n", d.Hostname)
				fmt.Fprintf(w, "Vendor:\t%s\n", d.Vendor)
				fmt.Fprintf(w, "Model:\t%s\n", d.Model)
				fmt.Fprintf(w, "OS Version:\t%s\n", d.OSVersion)
				fmt.Fprintf(w, "Mgmt IP:\t%s\n", d.MgmtIP)
				fmt.Fprintf(w, "Router-ID:\t%s\n", d.RouterID)
				fmt.Fprintf(w, "MPLS LSR-ID:\t%s\n", d.MPLSLsrID)
				fmt.Fprintf(w, "Last Seen:\t%s\n", d.LastSeen.Format("2006-01-02 15:04:05"))
				return w.Flush()
			}
			devices, err := db.ListDevices()
			if err != nil {
				return err
			}
			if len(devices) == 0 {
				fmt.Println("No devices found. Use 'nethelper watch ingest <file>' to import logs.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tHOSTNAME\tVENDOR\tMODEL\tMGMT IP\tLAST SEEN\n")
			for _, d := range devices {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					d.ID, d.Hostname, d.Vendor, d.Model, d.MgmtIP, d.LastSeen.Format("2006-01-02 15:04"))
			}
			return w.Flush()
		},
	}
}

func newShowInterfaceCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "interface",
		Short: "Show interface information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			ifaces, err := db.GetInterfaces(deviceID)
			if err != nil {
				return err
			}
			if len(ifaces) == 0 {
				fmt.Println("No interfaces found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "NAME\tTYPE\tSTATUS\tIP\tDESCRIPTION\n")
			for _, i := range ifaces {
				ip := i.IPAddress
				if ip != "" && i.Mask != "" {
					ip = ip + "/" + i.Mask
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", i.Name, i.Type, i.Status, ip, i.Description)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newShowRouteCmd() *cobra.Command {
	var deviceID, prefix, protocol string
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Show routing table (RIB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetRIBEntries(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No RIB entries found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PREFIX\tPROTOCOL\tNEXT-HOP\tINTERFACE\tPREF\tMETRIC\tVRF\n")
			for _, e := range entries {
				if prefix != "" && e.Prefix != prefix {
					continue
				}
				if protocol != "" && e.Protocol != protocol {
					continue
				}
				fmt.Fprintf(w, "%s/%d\t%s\t%s\t%s\t%d\t%d\t%s\n",
					e.Prefix, e.MaskLen, e.Protocol, e.NextHop, e.OutgoingInterface, e.Preference, e.Metric, e.VRF)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	cmd.Flags().StringVar(&prefix, "prefix", "", "filter by prefix")
	cmd.Flags().StringVar(&protocol, "protocol", "", "filter by protocol")
	return cmd
}

func newShowFIBCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "fib",
		Short: "Show forwarding table (FIB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetFIBEntries(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No FIB entries found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PREFIX\tNEXT-HOP\tINTERFACE\tLABEL-ACTION\tOUT-LABEL\tTUNNEL\n")
			for _, e := range entries {
				fmt.Fprintf(w, "%s/%d\t%s\t%s\t%s\t%s\t%s\n",
					e.Prefix, e.MaskLen, e.NextHop, e.OutgoingInterface, e.LabelAction, e.OutLabel, e.TunnelID)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newShowLabelCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Show label forwarding table (LFIB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetLFIBEntries(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No LFIB entries found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "IN-LABEL\tACTION\tOUT-LABEL\tNEXT-HOP\tINTERFACE\tFEC\tPROTOCOL\n")
			for _, e := range entries {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					e.InLabel, e.Action, e.OutLabel, e.NextHop, e.OutgoingInterface, e.FEC, e.Protocol)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newShowNeighborCmd() *cobra.Command {
	var deviceID, protocol string
	cmd := &cobra.Command{
		Use:   "neighbor",
		Short: "Show protocol neighbors",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetNeighbors(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No neighbors found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "PROTOCOL\tREMOTE-ID\tSTATE\tINTERFACE\tAREA\tAS\tUPTIME\n")
			for _, e := range entries {
				if protocol != "" && e.Protocol != protocol {
					continue
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
					e.Protocol, e.RemoteID, e.State, e.LocalInterface, e.AreaID, e.ASNumber, e.Uptime)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	cmd.Flags().StringVar(&protocol, "protocol", "", "filter by protocol")
	return cmd
}

func newShowTunnelCmd() *cobra.Command {
	var deviceID, tunnelType string
	cmd := &cobra.Command{
		Use:   "tunnel",
		Short: "Show TE/SR tunnels",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" {
				return fmt.Errorf("--device is required")
			}
			snapID, err := db.LatestSnapshotID(deviceID)
			if err != nil {
				return fmt.Errorf("no snapshots found for device %s", deviceID)
			}
			entries, err := db.GetTunnels(deviceID, snapID)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No tunnels found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "NAME\tTYPE\tSTATE\tDESTINATION\tBINDING-SID\tBW\n")
			for _, e := range entries {
				if tunnelType != "" && e.Type != tunnelType {
					continue
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
					e.TunnelName, e.Type, e.State, e.Destination, e.BindingSID, e.SignaledBW)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	cmd.Flags().StringVar(&tunnelType, "type", "", "filter by type")
	return cmd
}
