package analyzer

// Result is the outcome of analyzing a single page.
type Result struct {
	URL          string    `json:"url"`
	HTMLVersion  string    `json:"html_version"`
	Title        string    `json:"title"`
	Headings     Headings  `json:"headings"`
	Links        LinkStats `json:"links"`
	HasLoginForm bool      `json:"has_login_form"`
}

// Headings holds the number of headings found at each level.
type Headings struct {
	H1 int `json:"h1"`
	H2 int `json:"h2"`
	H3 int `json:"h3"`
	H4 int `json:"h4"`
	H5 int `json:"h5"`
	H6 int `json:"h6"`
}

// LinkStats counts the links on a page.
type LinkStats struct {
	Internal     int `json:"internal"`
	External     int `json:"external"`
	Checked      int `json:"checked"`
	Inaccessible int `json:"inaccessible"`
}
