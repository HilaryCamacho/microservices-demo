package main

import (
	"encoding/json"
	"os"
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
