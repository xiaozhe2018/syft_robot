package model

type Welfare struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Age         int        `json:"age"`
	Height      int        `json:"height"`
	Weight      int        `json:"weight"`
	Description string     `json:"description"`
	Photos      []string   `json:"photos"`
	Videos      []string   `json:"videos"`
	Packages    []Package  `json:"packages"`
}

type Package struct {
	Name     string `json:"name"`
	Price    int    `json:"price"`
	Duration string `json:"duration"`
	Times    int    `json:"times"`
	Note     string `json:"note,omitempty"`
}

type WelfareListResponse struct {
	Items []Welfare `json:"items"`
}

type WelfareListItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Photo  string `json:"photo"`
}

type WelfareDetailResponse struct {
	Welfare
}

type WelfareOrderRequest struct {
	WelfareID string `json:"welfare_id"`
	Address   string `json:"address"`
}

type WelfareOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
} 