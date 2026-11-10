package main

import (
	"sort"
	"strings"
	"sync"
	"time"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/frontend/genproto"
)

// Historial de pedidos por sesión (cookie del navegador), guardado en la memoria del frontend.
// El frontend corre con una réplica: el historial se conserva mientras el pod siga vivo y se
// borra si se reinicia o se redespliega. Nunca se guardan el número completo de la tarjeta ni el
// CVV: solo los últimos 4 dígitos.

const maxOrdersPerSession = 20

type orderItemView struct {
	ProductID string
	Name      string
	Picture   string
	Quantity  int32
	Cost      *pb.Money // precio unitario en la moneda del pedido
}

type storedOrder struct {
	ID           string
	TrackingID   string
	PlacedAt     time.Time
	Items        []orderItemView
	ShippingCost *pb.Money
	Total        *pb.Money
	ShipTo       string
	CardLast4    string
}

// Etapas del pedido. Avanzan con el tiempo para que se pueda ver el proceso durante la demo.
var orderStages = []struct {
	After       time.Duration
	Label       string
	Description string
}{
	{0, "Confirmed", "Payment received and order confirmed."},
	{1 * time.Minute, "Preparing", "We are packing your pet's products."},
	{3 * time.Minute, "Shipped", "Your order is on its way."},
	{6 * time.Minute, "Delivered", "Your order was delivered."},
}

type orderStep struct {
	Label       string
	Description string
	At          time.Time
	Done        bool
	Current     bool
}

// steps devuelve la línea de tiempo del pedido según el tiempo transcurrido desde la compra.
func (o *storedOrder) steps(now time.Time) []orderStep {
	elapsed := now.Sub(o.PlacedAt)
	current := 0
	for i, stage := range orderStages {
		if elapsed >= stage.After {
			current = i
		}
	}
	steps := make([]orderStep, len(orderStages))
	for i, stage := range orderStages {
		steps[i] = orderStep{
			Label:       stage.Label,
			Description: stage.Description,
			At:          o.PlacedAt.Add(stage.After),
			Done:        i <= current,
			Current:     i == current,
		}
	}
	return steps
}

// status devuelve la etapa actual del pedido.
func (o *storedOrder) status(now time.Time) string {
	for _, step := range o.steps(now) {
		if step.Current {
			return step.Label
		}
	}
	return orderStages[0].Label
}

type orderStore struct {
	mu        sync.Mutex
	bySession map[string][]*storedOrder
}

func newOrderStore() *orderStore {
	return &orderStore{bySession: make(map[string][]*storedOrder)}
}

var orders = newOrderStore()

// add guarda un pedido de la sesión. Conserva los más recientes (máximo maxOrdersPerSession).
func (s *orderStore) add(sessionID string, order *storedOrder) {
	if sessionID == "" || order == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := append(s.bySession[sessionID], order)
	if len(list) > maxOrdersPerSession {
		list = list[len(list)-maxOrdersPerSession:]
	}
	s.bySession[sessionID] = list
}

// list devuelve los pedidos de la sesión, del más reciente al más antiguo.
func (s *orderStore) list(sessionID string) []*storedOrder {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]*storedOrder(nil), s.bySession[sessionID]...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].PlacedAt.After(out[j].PlacedAt) })
	return out
}

// get devuelve un pedido solo si pertenece a la sesión: nadie puede ver pedidos de otra persona.
func (s *orderStore) get(sessionID, orderID string) *storedOrder {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, order := range s.bySession[sessionID] {
		if order.ID == orderID {
			return order
		}
	}
	return nil
}

// cardLast4 deja solo los últimos 4 dígitos de la tarjeta.
func cardLast4(ccNumber string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, ccNumber)
	if len(digits) < 4 {
		return ""
	}
	return digits[len(digits)-4:]
}

// orderSummary es la vista de un pedido para las plantillas.
type orderSummary struct {
	*storedOrder
	Status   string
	Steps    []orderStep
	Finished bool
}

func summarizeOrder(order *storedOrder, now time.Time) orderSummary {
	status := order.status(now)
	return orderSummary{
		storedOrder: order,
		Status:      status,
		Steps:       order.steps(now),
		Finished:    status == orderStages[len(orderStages)-1].Label,
	}
}
