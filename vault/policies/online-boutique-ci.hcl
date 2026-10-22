# Política del pipeline de CI/CD: solo lectura de los secretos de la aplicación.
# No puede escribir, borrar ni listar otras rutas.
path "secret/data/online-boutique/*" {
  capabilities = ["read"]
}

# Consultar su propio token (TTL restante). Permite al pipeline distinguir un token
# expirado de una política sin permiso y avisar antes de que venza.
path "auth/token/lookup-self" {
  capabilities = ["read"]
}
