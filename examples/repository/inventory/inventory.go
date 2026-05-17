package inventory

type StockLevel struct {
	SKU      string
	Quantity int
	Reserved int
}

func (s *StockLevel) Available() int {
	return s.Quantity - s.Reserved
}

func (s *StockLevel) Reserve(qty int) bool {
	if qty > s.Available() {
		return false
	}
	s.Reserved += qty
	return true
}

func (s *StockLevel) Restock(qty int) {
	s.Quantity += qty
}
