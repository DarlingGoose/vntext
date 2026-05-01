package game

type RuntimeStatus struct {
	WinePath           string   `json:"wine_path,omitempty"`
	WineBootPath       string   `json:"wineboot_path,omitempty"`
	SteamPath          string   `json:"steam_path,omitempty"`
	SteamRoot          string   `json:"steam_root,omitempty"`
	AvailableProton    []string `json:"available_proton,omitempty"`
	SelectedProtonPath string   `json:"selected_proton_path,omitempty"`
}
