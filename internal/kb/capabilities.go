package kb

type Capabilities struct {
	Dense  bool `json:"dense"`
	Sparse bool `json:"sparse"`
	Filter bool `json:"filter"`
	Hybrid bool `json:"hybrid"`
}

func DefaultCapabilities() Capabilities {
	return Capabilities{Dense: true, Sparse: true, Filter: true, Hybrid: true}
}
