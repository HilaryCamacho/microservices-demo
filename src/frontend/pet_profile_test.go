package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/frontend/genproto"
)

func TestNormalizePetProfile(t *testing.T) {
	tests := []struct {
		name                        string
		petType, petAge, petSize    string
		wantType, wantAge, wantSize string
	}{
		{"perfil válido", "dog", "puppy", "large", "dog", "puppy", "large"},
		{"edad de otra mascota", "dog", "kitten", "small", "dog", "", "small"},
		{"tamaño no disponible", "mouse", "young", "large", "mouse", "young", ""},
		{"mascota desconocida", "fish", "adult", "small", "", "", ""},
		{"sin mascota", "", "adult", "small", "", "", ""},
		{"sin perfil", "", "", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotAge, gotSize := normalizePetProfile(tt.petType, tt.petAge, tt.petSize)
			if gotType != tt.wantType || gotAge != tt.wantAge || gotSize != tt.wantSize {
				t.Errorf("normalizePetProfile(%q, %q, %q) = (%q, %q, %q), se esperaba (%q, %q, %q)",
					tt.petType, tt.petAge, tt.petSize, gotType, gotAge, gotSize, tt.wantType, tt.wantAge, tt.wantSize)
			}
		})
	}
}

func TestProductMatchesPetProfile(t *testing.T) {
	toy := &pb.Product{PetTypes: []string{"dog"}, PetAges: []string{"puppy"}, PetSizes: []string{"small", "medium"}}
	tests := []struct {
		name                     string
		petType, petAge, petSize string
		want                     bool
	}{
		{"sin perfil muestra todo", "", "", "", true},
		{"coincide", "dog", "puppy", "small", true},
		{"otra mascota", "cat", "", "", false},
		{"otra edad", "dog", "adult", "", false},
		{"otro tamaño", "dog", "puppy", "large", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := productMatchesPetProfile(toy, tt.petType, tt.petAge, tt.petSize); got != tt.want {
				t.Errorf("productMatchesPetProfile(%q, %q, %q) = %v, se esperaba %v", tt.petType, tt.petAge, tt.petSize, got, tt.want)
			}
		})
	}
}

// Cada opción que ofrece el formulario debe tener al menos un producto en el catálogo,
// para que ninguna selección válida deje la tienda vacía.
func TestPetProfileOptionsMatchCatalog(t *testing.T) {
	data, err := os.ReadFile("../productcatalogservice/products.json")
	if err != nil {
		t.Fatalf("no se pudo leer el catálogo: %v", err)
	}
	var catalog struct {
		Products []struct {
			Name     string   `json:"name"`
			PetTypes []string `json:"petTypes"`
			PetAges  []string `json:"petAges"`
			PetSizes []string `json:"petSizes"`
		} `json:"products"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("catálogo inválido: %v", err)
	}

	for petType, options := range petProfileOptions {
		for _, age := range options.Ages {
			found := false
			for _, p := range catalog.Products {
				if containsString(p.PetTypes, petType) && containsString(p.PetAges, age.Value) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ningún producto para %s + edad %q", petType, age.Value)
			}
		}
		for _, size := range options.Sizes {
			found := false
			for _, p := range catalog.Products {
				if containsString(p.PetTypes, petType) && containsString(p.PetSizes, size.Value) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ningún producto para %s + tamaño %q", petType, size.Value)
			}
		}
	}
}

func testCatalog() []*pb.Product {
	return []*pb.Product{
		{Id: "puppy-chew-toy", Name: "Puppy Chew Toy", PetTypes: []string{"dog"}, PetAges: []string{"puppy"}, PetSizes: []string{"small", "medium"}},
		{Id: "kitten-plush-toy", Name: "Kitten Plush Toy", PetTypes: []string{"cat"}, PetAges: []string{"kitten"}, PetSizes: []string{"small"}},
		{Id: "adult-dog-ball", Name: "Adult Dog Ball", PetTypes: []string{"dog"}, PetAges: []string{"adult"}, PetSizes: []string{"medium", "large"}},
		{Id: "pet-blanket", Name: "Cozy Pet Blanket", PetTypes: []string{"dog", "cat"}, PetAges: []string{"adult"}, PetSizes: []string{"small"}},
		{Id: "mouse-treat-mix", Name: "Mouse Treat Mix", PetTypes: []string{"mouse"}, PetAges: []string{"young"}, PetSizes: []string{"small"}},
	}
}

func productIDs(products []*pb.Product) []string {
	ids := make([]string, len(products))
	for i, p := range products {
		ids[i] = p.GetId()
	}
	return ids
}

func TestPetLabelAndEmoji(t *testing.T) {
	for value, want := range map[string]string{"dog": "Dog", "kitten": "Kitten", "large": "Large", "desconocido": "desconocido"} {
		if got := petLabel(value); got != want {
			t.Errorf("petLabel(%q) = %q, se esperaba %q", value, got, want)
		}
	}
	if petEmoji("cat") != "🐱" || petEmoji("fish") != "🐾" {
		t.Errorf("petEmoji no devuelve los íconos esperados")
	}
}

func TestCatalogAds(t *testing.T) {
	ads := []*pb.Ad{
		{RedirectUrl: "/product/2ZYFJ3GM2N", Text: "Producto que ya no existe"},
		{RedirectUrl: "/product/pet-blanket", Text: "Producto del catálogo"},
	}
	valid := catalogAds(ads, testCatalog())
	if len(valid) != 1 || valid[0].GetRedirectUrl() != "/product/pet-blanket" {
		t.Errorf("catalogAds debería dejar solo el anuncio de pet-blanket, quedó %v", valid)
	}
}

func TestPetAd(t *testing.T) {
	for i := 0; i < 20; i++ {
		ad := petAd(testCatalog(), "mouse", "", "")
		if ad.GetRedirectUrl() != "/product/mouse-treat-mix" {
			t.Fatalf("con perfil de ratón el anuncio debería ser de mouse-treat-mix, fue %q", ad.GetRedirectUrl())
		}
	}
	if ad := petAd(testCatalog(), "", "", ""); ad == nil {
		t.Errorf("sin perfil debería anunciar cualquier producto del catálogo")
	}
	if ad := petAd(nil, "dog", "", ""); ad != nil {
		t.Errorf("sin catálogo no debería haber anuncio")
	}
}

func TestPetRecommendations(t *testing.T) {
	catalog := testCatalog()
	recs := []*pb.Product{catalog[1], catalog[4], catalog[2]} // cat, mouse, dog

	got := productIDs(petRecommendations(recs, catalog, []string{"dog"}, []string{"puppy-chew-toy"}))
	want := []string{"adult-dog-ball", "pet-blanket", "kitten-plush-toy", "mouse-treat-mix"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("perro: %v, se esperaba %v (primero perro, sin el producto actual, máximo 4)", got, want)
	}

	if got := petRecommendations(recs, catalog, nil, nil); len(got) != len(recs) {
		t.Errorf("sin mascota deberían quedar las recomendaciones originales, quedaron %d", len(got))
	}

	if got := productIDs(petRecommendations(nil, catalog, []string{"cat"}, nil)); strings.Join(got, ",") != "kitten-plush-toy,pet-blanket" {
		t.Errorf("sin recomendaciones del servicio debería completar con el catálogo: %v", got)
	}
}

// Renderiza inicio y producto con datos reales para detectar errores de las plantillas
// (funciones, tipos) que no aparecen al solo parsearlas.
func TestPetTemplatesRender(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	price := &pb.Money{CurrencyCode: "USD", Units: 12}
	p := testCatalog()[0]
	type productView struct {
		Item  *pb.Product
		Price *pb.Money
	}

	var home bytes.Buffer
	err := templates.ExecuteTemplate(&home, "home", injectCommonTemplateData(r, map[string]interface{}{
		"show_currency": true,
		"currencies":    []string{"USD"},
		"products":      []productView{{p, price}},
		"cart_size":     0,
		"pet_type":      "dog",
		"pet_age":       "puppy",
		"pet_size":      "small",
		"pet_options":   petProfileOptions,
		"ad":            petAd(testCatalog(), "dog", "", ""),
	}))
	if err != nil {
		t.Fatalf("no se pudo renderizar home: %v", err)
	}
	for _, want := range []string{"Showing products for", "Clear filter", "pet-tag", "🐶 Dog", "PET_OPTIONS"} {
		if !strings.Contains(home.String(), want) {
			t.Errorf("home no contiene %q", want)
		}
	}

	var product bytes.Buffer
	err = templates.ExecuteTemplate(&product, "product", injectCommonTemplateData(r, map[string]interface{}{
		"show_currency":   true,
		"currencies":      []string{"USD"},
		"product":         productView{p, price},
		"recommendations": petRecommendations(nil, testCatalog(), []string{"dog"}, []string{p.GetId()}),
		"cart_size":       0,
	}))
	if err != nil {
		t.Fatalf("no se pudo renderizar product: %v", err)
	}
	for _, want := range []string{"Ideal for", "🐶 Dog", "Puppy", "Small, Medium", "Adult Dog Ball"} {
		if !strings.Contains(product.String(), want) {
			t.Errorf("product no contiene %q", want)
		}
	}
}
