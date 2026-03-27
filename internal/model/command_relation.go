package model

// CommandRelation describes the relationship between two CLI commands.
// SourceCmd produces output containing fields that can be used to construct
// or parameterise TargetCmd.
type CommandRelation struct {
	SourceCmd    string            // Source command pattern, e.g. "display bgp peer"
	TargetCmd    string            // Target command pattern, e.g. "display bgp peer {ip} verbose"
	RelationType string            // "reference" | "context" | "validation"
	FieldMapping map[string]string // e.g. {"Peer": "{ip}"}
	Description  string
}

// TroubleshootScenario describes a troubleshooting workflow as an ordered
// chain of diagnostic commands across multiple vendors.
type TroubleshootScenario struct {
	ID       string
	Name     string
	Domain   string // "ip" | "label" | "vpn" | "te" | "silent_drop"
	Triggers []string
	Steps    []TroubleshootStep
}

// TroubleshootStep describes a single step in a troubleshooting chain.
type TroubleshootStep struct {
	Order     int
	Purpose   string
	Commands  map[string]string // vendor → command template
	ParamFrom string            // Field name from a previous step's output
	Decisions []DecisionBranch
}

// DecisionBranch describes a conditional branch within a troubleshooting step.
type DecisionBranch struct {
	Condition  string
	FieldCheck string
	ValueMatch string // supports regex
	NextStep   int    // 0 = conclusion (terminal)
	Conclusion string
}
