package agent

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/plan"
	"github.com/xavierli/nethelper/internal/store"
)

// Tool represents an agent-callable tool.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{} // JSON Schema
	Handler     func(args map[string]interface{}) (string, error)
}

// ToLLMToolDef converts to LLM request format.
func (t Tool) ToLLMToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        t.Name,
		Description: t.Description,
		Parameters:  t.Parameters,
	}
}

// Registry holds all available tools.
type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry { return &Registry{tools: make(map[string]Tool)} }

func (r *Registry) Register(t Tool) {
	if _, exists := r.tools[t.Name]; !exists {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) ToolDefs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].ToLLMToolDef())
	}
	return defs
}

// RegisterNethelperTools registers all nethelper tools into the agent registry.
func RegisterNethelperTools(reg *Registry, db *store.DB, pipeline *parser.Pipeline) {
	// Helper for JSON Schema params
	obj := func(props map[string]interface{}, required []string) map[string]interface{} {
		schema := map[string]interface{}{"type": "object", "properties": props}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema
	}
	strProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}

	// show_devices
	reg.Register(Tool{
		Name:        "show_devices",
		Description: "List all network devices with hostname, vendor, OS version",
		Parameters:  obj(nil, nil),
		Handler: func(args map[string]interface{}) (string, error) {
			devices, err := db.ListDevices()
			if err != nil {
				return "", err
			}
			data, _ := json.MarshalIndent(devices, "", "  ")
			return string(data), nil
		},
	})

	// show_device
	reg.Register(Tool{
		Name:        "show_device",
		Description: "Get details for a specific device",
		Parameters:  obj(map[string]interface{}{"device_id": strProp("Device ID")}, []string{"device_id"}),
		Handler: func(args map[string]interface{}) (string, error) {
			id, _ := args["device_id"].(string)
			dev, err := db.GetDevice(id)
			if err != nil {
				return "", err
			}
			data, _ := json.MarshalIndent(dev, "", "  ")
			return string(data), nil
		},
	})

	// show_interfaces
	reg.Register(Tool{
		Name:        "show_interfaces",
		Description: "List interfaces for a device",
		Parameters:  obj(map[string]interface{}{"device_id": strProp("Device ID")}, []string{"device_id"}),
		Handler: func(args map[string]interface{}) (string, error) {
			id, _ := args["device_id"].(string)
			ifaces, err := db.GetInterfaces(id)
			if err != nil {
				return "", err
			}
			data, _ := json.MarshalIndent(ifaces, "", "  ")
			return string(data), nil
		},
	})

	// show_bgp_peers (summary)
	reg.Register(Tool{
		Name:        "show_bgp_peers",
		Description: "List BGP peer groups for a device (summary with counts)",
		Parameters:  obj(map[string]interface{}{"device_id": strProp("Device ID")}, []string{"device_id"}),
		Handler: func(args map[string]interface{}) (string, error) {
			id, _ := args["device_id"].(string)
			peers, err := db.GetLatestBGPPeers(id)
			if err != nil {
				return "", err
			}
			// Return summary by group
			groups := make(map[string]int)
			for _, p := range peers {
				groups[p.PeerGroup]++
			}
			data, _ := json.MarshalIndent(groups, "", "  ")
			return string(data), nil
		},
	})

	// show_neighbors
	reg.Register(Tool{
		Name:        "show_neighbors",
		Description: "List protocol neighbors for a device",
		Parameters:  obj(map[string]interface{}{"device_id": strProp("Device ID")}, []string{"device_id"}),
		Handler: func(args map[string]interface{}) (string, error) {
			id, _ := args["device_id"].(string)
			neighbors, err := db.GetLatestNeighbors(id)
			if err != nil {
				return "", err
			}
			data, _ := json.MarshalIndent(neighbors, "", "  ")
			return string(data), nil
		},
	})

	// plan_isolate
	reg.Register(Tool{
		Name:        "plan_isolate",
		Description: "Generate a device isolation change plan",
		Parameters:  obj(map[string]interface{}{"device_id": strProp("Device ID")}, []string{"device_id"}),
		Handler: func(args map[string]interface{}) (string, error) {
			id, _ := args["device_id"].(string)
			topo, err := plan.BuildTopology(db, id)
			if err != nil {
				return "", err
			}
			p := plan.GenerateIsolationPlanV2(topo)
			return plan.RenderMarkdown(p), nil
		},
	})

	// plan_upgrade
	reg.Register(Tool{
		Name:        "plan_upgrade",
		Description: "Generate a device upgrade change plan",
		Parameters: obj(map[string]interface{}{
			"device_id": strProp("Device ID"),
			"version":   strProp("Target version"),
			"file":      strProp("Firmware file name"),
		}, []string{"device_id", "version", "file"}),
		Handler: func(args map[string]interface{}) (string, error) {
			id, _ := args["device_id"].(string)
			version, _ := args["version"].(string)
			file, _ := args["file"].(string)
			topo, err := plan.BuildTopology(db, id)
			if err != nil {
				return "", err
			}
			p := plan.GenerateUpgradePlan(topo, plan.UpgradeParams{TargetVersion: version, FirmwareFile: file})
			return plan.RenderMarkdown(p), nil
		},
	})

	// search_log
	reg.Register(Tool{
		Name:        "search_log",
		Description: "Search troubleshooting notes for past experiences",
		Parameters:  obj(map[string]interface{}{"query": strProp("Search keywords")}, []string{"query"}),
		Handler: func(args map[string]interface{}) (string, error) {
			query, _ := args["query"].(string)
			logs, err := db.SearchTroubleshootLogs(query)
			if err != nil {
				return "No results found", nil
			}
			data, _ := json.MarshalIndent(logs, "", "  ")
			return string(data), nil
		},
	})

	// note_add
	reg.Register(Tool{
		Name:        "note_add",
		Description: "Record a troubleshooting experience",
		Parameters: obj(map[string]interface{}{
			"device_id":     strProp("Related device ID (optional)"),
			"symptom":       strProp("Problem symptom"),
			"commands_used": strProp("Commands used during troubleshooting"),
			"findings":      strProp("Key findings"),
			"resolution":    strProp("How it was resolved"),
			"tags":          strProp("Comma-separated tags"),
		}, []string{"symptom", "resolution"}),
		Handler: func(args map[string]interface{}) (string, error) {
			log := model.TroubleshootLog{
				DeviceID:     getStr(args, "device_id"),
				Symptom:      getStr(args, "symptom"),
				CommandsUsed: getStr(args, "commands_used"),
				Findings:     getStr(args, "findings"),
				Resolution:   getStr(args, "resolution"),
				Tags:         getStr(args, "tags"),
			}
			_, err := db.InsertTroubleshootLog(log)
			if err != nil {
				return "", err
			}
			return "Experience recorded successfully", nil
		},
	})

	// watch_ingest
	reg.Register(Tool{
		Name:        "watch_ingest",
		Description: "Import a log file into nethelper",
		Parameters:  obj(map[string]interface{}{"file_path": strProp("Path to log file")}, []string{"file_path"}),
		Handler: func(args map[string]interface{}) (string, error) {
			path, _ := args["file_path"].(string)
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			result, err := pipeline.Ingest(path, string(data))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Ingested: %d devices, %d blocks parsed", result.DevicesFound, result.BlocksParsed), nil
		},
	})
}

func getStr(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}
