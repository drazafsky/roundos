// Package spec contains the public, pure-data RUI v1 syntax tree. A caller
// may construct it programmatically or parse validated RUI YAML using the
// internal compiler. Neither the schema nor the AST executes application code.
package spec

const Version = "roundos.rui/v1"

// Pos identifies the one-based source location of a syntax node. Compiler
// diagnostics use these positions; positions have no effect on rendering.
type Pos struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Value is either a literal T or a named, typed SDK binding, never both.
// Present distinguishes an explicit zero value from an omitted field.
type Value[T any] struct {
	Literal T      `json:"literal,omitempty"`
	Binding string `json:"binding,omitempty"`
	Bound   bool   `json:"bound,omitempty"`
	Present bool   `json:"present"`
	Pos     Pos    `json:"pos"`
}

type Document struct {
	Schema      string      `json:"schema"`
	Application Application `json:"application"`
	Screens     []Screen    `json:"screens"`
	Dialogs     []Dialog    `json:"dialogs,omitempty"`
	Pos         Pos         `json:"pos"`
}

type Application struct {
	ID  string `json:"id"`
	Pos Pos    `json:"pos"`
}

type Screen struct {
	ID       string         `json:"id"`
	Title    Value[string]  `json:"title"`
	Layout   string         `json:"layout"`
	Hero     *Value[string] `json:"hero,omitempty"`
	Subtitle *Value[string] `json:"subtitle,omitempty"`
	Groups   []Group        `json:"groups"`
	Pos      Pos            `json:"pos"`
}

type Group struct {
	ID      string         `json:"id"`
	Title   *Value[string] `json:"title,omitempty"`
	Widgets []Widget       `json:"widgets"`
	Pos     Pos            `json:"pos"`
}

type Widget struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Label       *Value[string] `json:"label,omitempty"`
	Detail      *Value[string] `json:"detail,omitempty"`
	Variant     string         `json:"variant,omitempty"`
	Value       *Value[int]    `json:"value,omitempty"`
	Checked     *Value[bool]   `json:"checked,omitempty"`
	Min         *int           `json:"min,omitempty"`
	Max         *int           `json:"max,omitempty"`
	MinBinding  string         `json:"minBinding,omitempty"`
	MaxBinding  string         `json:"maxBinding,omitempty"`
	Step        *int           `json:"step,omitempty"`
	Wrap        *bool          `json:"wrap,omitempty"`
	EnabledWhen *Value[bool]   `json:"enabledWhen,omitempty"`
	VisibleWhen *Value[bool]   `json:"visibleWhen,omitempty"`
	OnPress     string         `json:"onPress,omitempty"`
	OnChange    string         `json:"onChange,omitempty"`
	OnCommit    string         `json:"onCommit,omitempty"`
	Navigate    string         `json:"navigate,omitempty"`
	Pos         Pos            `json:"pos"`
}

type Dialog struct {
	ID           string         `json:"id"`
	Title        Value[string]  `json:"title"`
	Body         *Value[string] `json:"body,omitempty"`
	ConfirmLabel string         `json:"confirmLabel"`
	CancelLabel  string         `json:"cancelLabel"`
	OnConfirm    string         `json:"onConfirm"`
	OnCancel     string         `json:"onCancel"`
	Destructive  bool           `json:"destructive"`
	Pos          Pos            `json:"pos"`
}

// BindingType enumerates the only v1 state binding categories. TimePicker is
// a minute-of-day integer and DatePicker is a UTC epoch-day integer.
type BindingType string

const (
	StringBinding BindingType = "string"
	IntBinding    BindingType = "int"
	BoolBinding   BindingType = "bool"
)

// Manifest is an M1-level static contract for future M2/M3 tooling. Actions
// and bindings are deduplicated and sorted by the validator.
type Manifest struct {
	Bindings map[string]BindingType `json:"bindings"`
	Actions  []Action               `json:"actions"`
}

type Action struct {
	Name      string `json:"name"`
	ValueType string `json:"valueType"`
}
