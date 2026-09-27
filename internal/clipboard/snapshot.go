package clipboard

import "beam/internal/history"

func (s *Service) Snapshot() (*history.Item, error) {
	if s.p == nil {
		return nil, nil
	}
	it, err := s.p.Read()
	if err != nil || it == nil {
		return nil, err
	}
	fp := fingerprint(it)
	if fp == "" || fp == s.last {
		return nil, nil
	}
	latest, _ := s.store.GetLatest()
	if latest != nil && sameClip(latest, it) {
		s.last = fp
		return nil, nil
	}
	saved, err := s.persist(it)
	if err != nil {
		return nil, err
	}
	s.last = fp
	return saved, nil
}

func sameClip(h *history.Item, it *Item) bool {
	if h.Kind != it.Kind {
		return false
	}
	if it.Kind == history.KindText {
		return h.Text == it.Text
	}
	return h.Filename == it.Filename && h.Size == int64(len(it.Data))
}
