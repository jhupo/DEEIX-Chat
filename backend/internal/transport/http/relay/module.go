package relay

import apprelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/relay"

type Module struct{ Handler *Handler }

func NewModule(service *apprelay.Service) *Module { return &Module{Handler: NewHandler(service)} }
