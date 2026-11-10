package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/frontend/genproto"
)

func testOrder(id string, placedAt time.Time) *storedOrder {
	return &storedOrder{
		ID:           id,
		TrackingID:   "TRACK-" + id,
		PlacedAt:     placedAt,
		Items:        []orderItemView{{ProductID: "puppy-chew-toy", Name: "Puppy Chew Toy", Picture: "/static/img/products/puppy-chew-toy.jpg", Quantity: 2, Cost: &pb.Money{CurrencyCode: "USD", Units: 12}}},
		ShippingCost: &pb.Money{CurrencyCode: "USD", Units: 8},
		Total:        &pb.Money{CurrencyCode: "USD", Units: 32},
		ShipTo:       "Monterrey, NL, Mexico",
		CardLast4:    "1111",
	}
}

func TestOrderStatusProgress(t *testing.T) {
	placed := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	order := testOrder("A1", placed)
	tests := []struct {
		after time.Duration
		want  string
	}{
		{0, "Confirmed"},
		{90 * time.Second, "Preparing"},
		{4 * time.Minute, "Shipped"},
		{10 * time.Minute, "Delivered"},
	}
	for _, tt := range tests {
		now := placed.Add(tt.after)
		if got := order.status(now); got != tt.want {
			t.Errorf("a los %v el estado es %q, se esperaba %q", tt.after, got, tt.want)
		}
		done := 0
		for _, step := range order.steps(now) {
			if step.Done {
				done++
			}
		}
		if want := map[string]int{"Confirmed": 1, "Preparing": 2, "Shipped": 3, "Delivered": 4}[tt.want]; done != want {
			t.Errorf("a los %v hay %d etapas completadas, se esperaban %d", tt.after, done, want)
		}
	}
	if !summarizeOrder(order, placed.Add(time.Hour)).Finished {
		t.Errorf("un pedido entregado debería marcarse como terminado")
	}
}

func TestOrderStoreIsolatesSessions(t *testing.T) {
	store := newOrderStore()
	now := time.Now()
	store.add("ana", testOrder("A1", now.Add(-time.Hour)))
	store.add("ana", testOrder("A2", now))
	store.add("beto", testOrder("B1", now))
	store.add("", testOrder("X", now)) // sin sesión no se guarda

	list := store.list("ana")
	if len(list) != 2 || list[0].ID != "A2" || list[1].ID != "A1" {
		t.Fatalf("ana debería ver A2 y A1 (el más reciente primero), ve %v", list)
	}
	if store.get("beto", "A1") != nil {
		t.Errorf("beto no debería poder ver un pedido de ana")
	}
	if store.get("ana", "A1") == nil {
		t.Errorf("ana debería ver su pedido A1")
	}
	if len(store.list("")) != 0 {
		t.Errorf("no deberían guardarse pedidos sin sesión")
	}
}

func TestOrderStoreKeepsLatest(t *testing.T) {
	store := newOrderStore()
	start := time.Now()
	for i := 0; i < maxOrdersPerSession+5; i++ {
		store.add("ana", testOrder(fmt.Sprintf("O%d", i), start.Add(time.Duration(i)*time.Minute)))
	}
	list := store.list("ana")
	if len(list) != maxOrdersPerSession {
		t.Fatalf("se esperaban %d pedidos, hay %d", maxOrdersPerSession, len(list))
	}
	if list[0].ID != fmt.Sprintf("O%d", maxOrdersPerSession+4) || store.get("ana", "O0") != nil {
		t.Errorf("deberían quedar solo los pedidos más recientes")
	}
}

func TestCardLast4(t *testing.T) {
	for in, want := range map[string]string{"4111 1111 1111 1111": "1111", "5555-5555-5555-4444": "4444", "123": ""} {
		if got := cardLast4(in); got != want {
			t.Errorf("cardLast4(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

func TestOrderTemplatesRender(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/orders", nil)
	order := testOrder("abc-123", time.Now().Add(-4*time.Minute))

	var list bytes.Buffer
	if err := templates.ExecuteTemplate(&list, "orders", injectCommonTemplateData(r, map[string]interface{}{
		"orders": []orderSummary{summarizeOrder(order, time.Now())},
	})); err != nil {
		t.Fatalf("no se pudo renderizar orders: %v", err)
	}
	for _, want := range []string{"My Orders", "Order abc-123", "Shipped", "2 × Puppy Chew Toy", "/orders/abc-123"} {
		if !strings.Contains(list.String(), want) {
			t.Errorf("orders no contiene %q", want)
		}
	}

	var empty bytes.Buffer
	if err := templates.ExecuteTemplate(&empty, "orders", injectCommonTemplateData(r, map[string]interface{}{
		"orders": []orderSummary{},
	})); err != nil || !strings.Contains(empty.String(), "You haven't placed any orders yet") {
		t.Errorf("orders vacío debería invitar a comprar (err: %v)", err)
	}

	var detail bytes.Buffer
	if err := templates.ExecuteTemplate(&detail, "order_detail", injectCommonTemplateData(r, map[string]interface{}{
		"order": summarizeOrder(order, time.Now()),
	})); err != nil {
		t.Fatalf("no se pudo renderizar order_detail: %v", err)
	}
	for _, want := range []string{"Confirmed", "Preparing", "Shipped", "Delivered", "TRACK-abc-123", "card ending in 1111", "Monterrey, NL, Mexico", "window.location.reload"} {
		if !strings.Contains(detail.String(), want) {
			t.Errorf("order_detail no contiene %q", want)
		}
	}
}

// El carrito con errores vuelve a mostrar lo que el usuario escribió, pero nunca la tarjeta ni el CVV.
func TestCartRendersCheckoutErrors(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/cart", nil)
	p := testCatalog()[0]
	type cartItemView struct {
		Item     *pb.Product
		Quantity int32
		Price    *pb.Money
	}
	year := time.Now().Year()

	var cart bytes.Buffer
	err := templates.ExecuteTemplate(&cart, "cart", injectCommonTemplateData(r, map[string]interface{}{
		"currencies":        []string{"USD"},
		"cart_size":         1,
		"shipping_cost":     &pb.Money{CurrencyCode: "USD", Units: 8},
		"show_currency":     true,
		"total_cost":        pb.Money{CurrencyCode: "USD", Units: 20},
		"items":             []cartItemView{{p, 1, &pb.Money{CurrencyCode: "USD", Units: 12}}},
		"expiration_months": expirationMonths,
		"expiration_years":  []int{year, year + 1},
		"checkout":          checkoutForm{Email: "ana@example.com", City: "Monterrey", Month: 12, Year: year + 1},
		"checkout_errors":   []string{"The card number is not valid."},
	}))
	if err != nil {
		t.Fatalf("no se pudo renderizar cart: %v", err)
	}
	html := cart.String()
	for _, want := range []string{"The card number is not valid.", `value="ana@example.com"`, `value="Monterrey"`, "December", "4111 1111 1111 1111"} {
		if !strings.Contains(html, want) {
			t.Errorf("cart no contiene %q", want)
		}
	}
	for _, unwanted := range []string{"someone@example.com", "4432801561520454", `value="672"`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("cart todavía tiene el dato fijo %q", unwanted)
		}
	}
}
