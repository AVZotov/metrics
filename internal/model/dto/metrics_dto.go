package dto

import (
	"fmt"
	"strconv"

	models "github.com/AVZotov/metrics/internal/model"
)

type MetricsDTO struct {
	Name  string
	Value string
}

func (m *MetricsDTO) String() string {
	return fmt.Sprintf("%s:\t%s", m.Name, m.Value)
}

func GetMetricsDTOs(m []*models.Metrics) []MetricsDTO {
	dtos := make([]MetricsDTO, 0)
	for _, mm := range m {
		dtos = append(dtos, getMetricsDTO(mm))
	}
	return dtos
}

func getMetricsDTO(m *models.Metrics) MetricsDTO {
	if m == nil {
		return MetricsDTO{}
	}
	name := m.ID
	dto := MetricsDTO{
		Name: name,
	}
	switch m.MType {
	case models.Gauge:
		if m.Value != nil {
			dto.Value = strconv.FormatFloat(*m.Value, 'f', -1, 64)
		}
	case models.Counter:
		if m.Delta != nil {
			dto.Value = strconv.FormatInt(*m.Delta, 10)
		}
	}
	return dto
}
