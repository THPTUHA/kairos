package filepipe

import "context"

type pipe struct {
	context context.Context
	files   <-chan File
}

func (p pipe) Context() context.Context {
	return p.context
}

func (p pipe) Files() <-chan File {
	return p.files
}
func (p pipe) Pipe(stages ...Stage) Pipe {
	switch len(stages) {
	case 0:
		return p
	case 1:
		return makestage(stages[0], p.Context(), p.Files())
	default:
		return makestage(stages[0], p.Context(), p.Files()).Pipe(stages[1:]...)
	}
}

func (p pipe) Wait() error {
	var err error
	for f := range p.files {
		e := f.Close()
		if err == nil && e != nil {
			err = e
		}
	}
	return err
}

func (p pipe) Then(stages ...Stage) error {
	return p.Pipe(stages...).Wait()
}
