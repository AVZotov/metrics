package repository

import models "github.com/AVZotov/metrics/internal/model"

type Repository interface {
	Save(metrics *models.Metrics) error
}
