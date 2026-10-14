# Política del pipeline de CI/CD: solo lectura de los secretos de la aplicación.
# No puede escribir, borrar ni listar otras rutas.
path "secret/data/online-boutique/*" {
  capabilities = ["read"]
}
