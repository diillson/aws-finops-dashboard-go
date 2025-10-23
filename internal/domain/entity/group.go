package entity

// ProfileGroup representa uma unidade de trabalho para processamento.
// Pode ser um único perfil ou um grupo de perfis da mesma conta AWS
// que devem ser processados juntos quando a flag --combine é usada.
type ProfileGroup struct {
	// Identifier é o nome que será exibido na interface do usuário.
	// Para um perfil único, será o nome do perfil (ex: "default").
	// Para um grupo combinado, será uma lista dos perfis (ex: "dev, staging, prod").
	Identifier string

	// AccountID é o ID da conta AWS. É preenchido principalmente
	// quando IsCombined é true, para identificar o grupo.
	AccountID string

	// Profiles é a lista de nomes de perfis AWS reais que compõem este grupo.
	// Para um perfil único, conterá apenas um elemento.
	Profiles []string

	// IsCombined é um booleano que indica se este grupo representa
	// múltiplos perfis de uma mesma conta que devem ser agregados.
	IsCombined bool
}
