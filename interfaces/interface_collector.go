package interfaces

import "github.com/prometheus/client_golang/prometheus"

const prefix = "junos_interface_"

var (
	receiveBytesDesc   *prometheus.Desc
	receiveErrorsDesc  *prometheus.Desc
	receiveDropsDesc   *prometheus.Desc
	transmitBytesDesc  *prometheus.Desc
	transmitErrorsDesc *prometheus.Desc
	transmitDropsDesc  *prometheus.Desc
	adminStatusDesc    *prometheus.Desc
	operStatusDesc     *prometheus.Desc
	errorStatusDesc    *prometheus.Desc
)

func init() {
	l := []string{"target", "name", "description", "mac"}
	receiveBytesDesc = prometheus.NewDesc(prefix+"receive_bytes", "Received data in bytes", l, nil)
	receiveErrorsDesc = prometheus.NewDesc(prefix+"receive_errors", "Number of errors caused by incoming packets", l, nil)
	receiveDropsDesc = prometheus.NewDesc(prefix+"receive_drops", "Number of dropped incoming packets", l, nil)
	transmitBytesDesc = prometheus.NewDesc(prefix+"transmit_bytes", "Transmitted data in bytes", l, nil)
	transmitErrorsDesc = prometheus.NewDesc(prefix+"transmit_errors", "Number of errors caused by outgoing packets", l, nil)
	transmitDropsDesc = prometheus.NewDesc(prefix+"transmit_drops", "Number of dropped outgoing packets", l, nil)
	adminStatusDesc = prometheus.NewDesc(prefix+"admin_up", "Admin operational status", l, nil)
	operStatusDesc = prometheus.NewDesc(prefix+"up", "Interface operational status", l, nil)
	errorStatusDesc = prometheus.NewDesc(prefix+"error_status", "Admin and operational status differ", l, nil)
}

type InterfaceCollector struct {
}

func (*InterfaceCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- receiveBytesDesc
	ch <- receiveErrorsDesc
	ch <- receiveDropsDesc
	ch <- transmitBytesDesc
	ch <- transmitDropsDesc
	ch <- transmitErrorsDesc
	ch <- adminStatusDesc
	ch <- operStatusDesc
	ch <- errorStatusDesc
}

func (c *InterfaceCollector) Collect(datasource InterfaceStatsDatasource, ch chan<- prometheus.Metric, labelValues []string) error {
	stats, err := datasource.InterfaceStats()
	if err != nil {
		return err
	}

	for _, s := range stats {
		c.collectForInterface(s, ch, labelValues)
	}

	return nil
}

func (*InterfaceCollector) collectForInterface(s *InterfaceStats, ch chan<- prometheus.Metric, labelValues []string) {
	l := append(labelValues, []string{s.Name, s.Description, s.Mac}...)
	ch <- prometheus.MustNewConstMetric(receiveBytesDesc, prometheus.GaugeValue, s.ReceiveBytes, l...)
	ch <- prometheus.MustNewConstMetric(transmitBytesDesc, prometheus.GaugeValue, s.TransmitBytes, l...)

	if s.IsPhysical {
		adminUp := 0
		if s.AdminStatus {
			adminUp = 1
		}
		operUp := 0
		if s.OperStatus {
			operUp = 1
		}
		err := 0
		if s.ErrorStatus {
			err = 1
		}

		ch <- prometheus.MustNewConstMetric(adminStatusDesc, prometheus.GaugeValue, float64(adminUp), l...)
		ch <- prometheus.MustNewConstMetric(operStatusDesc, prometheus.GaugeValue, float64(operUp), l...)
		ch <- prometheus.MustNewConstMetric(errorStatusDesc, prometheus.GaugeValue, float64(err), l...)
		ch <- prometheus.MustNewConstMetric(transmitErrorsDesc, prometheus.GaugeValue, s.TransmitErrors, l...)
		ch <- prometheus.MustNewConstMetric(transmitDropsDesc, prometheus.GaugeValue, s.TransmitDrops, l...)
		ch <- prometheus.MustNewConstMetric(receiveErrorsDesc, prometheus.GaugeValue, s.ReceiveErrors, l...)
		ch <- prometheus.MustNewConstMetric(receiveDropsDesc, prometheus.GaugeValue, s.ReceiveDrops, l...)
	}
}
