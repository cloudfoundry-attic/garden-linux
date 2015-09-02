package layercake

import "github.com/pivotal-golang/lager"

type OvenCleaner struct {
	Cake
	Retainer

	Logger lager.Logger
}

func (g *OvenCleaner) Remove(id ID) error {
	log := g.Logger.Session("remove", lager.Data{"ID": id})
	log.Info("start")

	if g.Retainer.IsHeld(id) {
		log.Info("layer-is-held")
		return nil
	}

	img, err := g.Cake.Get(id)
	if err != nil {
		log.Error("get-image", err)
		return nil
	}

	if err := g.Cake.Remove(id); err != nil {
		return err
	}

	if img.Parent == "" {
		return nil
	}

	if leaf, err := g.Cake.IsLeaf(DockerImageID(img.Parent)); err == nil && leaf {
		return g.Remove(DockerImageID(img.Parent))
	}

	return nil
}
