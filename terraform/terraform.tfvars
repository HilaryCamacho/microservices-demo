name              = "online-boutique"
region            = "us-east-2"
github_repository = "HilaryCamacho/microservices-demo"

# El repositorio usa subjects OIDC inmutables (gh api repos/HilaryCamacho/microservices-demo/actions/oidc/customization/sub)
github_oidc_subject_prefix = "repo:HilaryCamacho@188712338/microservices-demo@1356741854"

# ARNs IAM de otros miembros del equipo que necesiten usar kubectl en el clúster
cluster_admin_arns = []
