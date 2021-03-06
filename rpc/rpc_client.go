package rpc

import (
	"encoding/xml"
	"fmt"
	"strconv"

	"log"

	"regexp"

	"github.com/czerwonk/junos_exporter/alarm"
	"github.com/czerwonk/junos_exporter/bgp"
	"github.com/czerwonk/junos_exporter/connector"
	"github.com/czerwonk/junos_exporter/environment"
	"github.com/czerwonk/junos_exporter/interface_diagnostics"
	"github.com/czerwonk/junos_exporter/interfaces"
	"github.com/czerwonk/junos_exporter/isis"
	"github.com/czerwonk/junos_exporter/ospf"
	"github.com/czerwonk/junos_exporter/route"
	"github.com/czerwonk/junos_exporter/routing_engine"
)

type RpcClient struct {
	conn        *connector.SshConnection
	debug       bool
	alarmFilter *regexp.Regexp
}

func NewClient(ssh *connector.SshConnection, debug bool, alarmFilter string) *RpcClient {
	rpc := &RpcClient{conn: ssh, debug: debug, alarmFilter: nil}

	if len(alarmFilter) > 0 {
		rpc.alarmFilter = regexp.MustCompile(alarmFilter)
	}

	return rpc
}

func (c *RpcClient) AlarmCounter() (*alarm.AlarmCounter, error) {
	red := 0
	yellow := 0

	cmds := []string{"show system alarms", "show chassis alarms"}

	for _, cmd := range cmds {
		var a = AlarmRpc{}
		err := c.runCommandAndParse(cmd, &a)
		if err != nil {
			return nil, err
		}

		for _, d := range a.Information.Details {
			if c.shouldFilterAlarm(&d) {
				continue
			}

			if d.Class == "Major" {
				red++
			} else if d.Class == "Minor" {
				yellow++
			}
		}
	}

	return &alarm.AlarmCounter{RedCount: float64(red), YellowCount: float64(yellow)}, nil
}

func (c *RpcClient) shouldFilterAlarm(alarm *AlarmDetails) bool {
	if c.alarmFilter == nil {
		return false
	}

	return c.alarmFilter.MatchString(alarm.Description) || c.alarmFilter.MatchString(alarm.Type)
}

func (c *RpcClient) InterfaceStats() ([]*interfaces.InterfaceStats, error) {
	var x = InterfaceRpc{}
	err := c.runCommandAndParse("show interfaces statistics detail", &x)
	if err != nil {
		return nil, err
	}

	stats := make([]*interfaces.InterfaceStats, 0)
	for _, phy := range x.Information.Interfaces {
		s := &interfaces.InterfaceStats{
			IsPhysical:     true,
			Name:           phy.Name,
			AdminStatus:    phy.AdminStatus == "up",
			OperStatus:     phy.OperStatus == "up",
			ErrorStatus:    !(phy.AdminStatus == phy.OperStatus),
			Description:    phy.Description,
			Mac:            phy.MacAddress,
			ReceiveDrops:   float64(phy.InputErrors.Drops),
			ReceiveErrors:  float64(phy.InputErrors.Errors),
			ReceiveBytes:   float64(phy.Stats.InputBytes),
			TransmitDrops:  float64(phy.OutputErrors.Drops),
			TransmitErrors: float64(phy.OutputErrors.Errors),
			TransmitBytes:  float64(phy.Stats.OutputBytes),
		}

		stats = append(stats, s)

		for _, log := range phy.LogicalInterfaces {
			sl := &interfaces.InterfaceStats{
				IsPhysical:    false,
				Name:          log.Name,
				Description:   log.Description,
				Mac:           phy.MacAddress,
				ReceiveBytes:  float64(log.Stats.InputBytes),
				TransmitBytes: float64(log.Stats.OutputBytes),
			}

			stats = append(stats, sl)
		}
	}

	return stats, nil
}

func (c *RpcClient) BgpSessions() ([]*bgp.BgpSession, error) {
	var x = BgpRpc{}
	err := c.runCommandAndParse("show bgp summary", &x)
	if err != nil {
		return nil, err
	}

	sessions := make([]*bgp.BgpSession, 0)
	for _, peer := range x.Information.Peers {
		s := &bgp.BgpSession{
			Ip:               peer.Ip,
			Up:               peer.State == "Established",
			Asn:              peer.Asn,
			Flaps:            float64(peer.Flaps),
			InputMessages:    float64(peer.InputMessages),
			OutputMessages:   float64(peer.OutputMessages),
			AcceptedPrefixes: float64(peer.Rib.AcceptedPrefixes),
			ActivePrefixes:   float64(peer.Rib.ActivePrefixes),
			ReceivedPrefixes: float64(peer.Rib.ReceivedPrefixes),
			RejectedPrefixes: float64(peer.Rib.RejectedPrefixes),
		}

		sessions = append(sessions, s)
	}

	return sessions, nil
}

func (c *RpcClient) OspfAreas() ([]*ospf.OspfArea, error) {
	var x = Ospf3Rpc{}
	err := c.runCommandAndParse("show ospf3 overview", &x)
	if err != nil {
		return nil, err
	}

	areas := make([]*ospf.OspfArea, 0)
	for _, area := range x.Information.Overview.Areas {
		a := &ospf.OspfArea{
			Name:      area.Name,
			Neighbors: float64(area.Neighbors.NeighborsUp),
		}

		areas = append(areas, a)
	}

	return areas, nil
}

func (c *RpcClient) IsisAdjancies() (*isis.IsisAdjacencies, error) {
	up := 0
	total := 0
	var x = IsisRpc{}
	err := c.runCommandAndParse("show isis adjacency", &x)
	if err != nil {
		return nil, err
	}

	for _, adjacency := range x.Information.Adjacencies {
		if adjacency.AdjacencyState == "Up" {
			up++
		}
		total++
	}

	return &isis.IsisAdjacencies{Up: float64(up), Total: float64(total)}, nil
}

func (c *RpcClient) RoutingTables() ([]*route.RoutingTable, error) {
	var x = RouteRpc{}
	err := c.runCommandAndParse("show route summary", &x)
	if err != nil {
		return nil, err
	}

	tables := make([]*route.RoutingTable, 0)
	for _, table := range x.Information.Tables {
		t := &route.RoutingTable{
			Name:         table.Name,
			MaxRoutes:    float64(table.MaxRoutes),
			ActiveRoutes: float64(table.ActiveRoutes),
			TotalRoutes:  float64(table.TotalRoutes),
			Protocols:    make([]*route.ProtocolRouteCount, 0),
		}

		for _, proto := range table.Protocols {
			p := &route.ProtocolRouteCount{
				Name:         proto.Name,
				Routes:       float64(proto.Routes),
				ActiveRoutes: float64(proto.ActiveRoutes),
			}

			t.Protocols = append(t.Protocols, p)
		}

		tables = append(tables, t)
	}

	return tables, nil
}

func (c *RpcClient) RouteEngineStats() (*routing_engine.RouteEngineStats, error) {
	var x = RoutingEngineRpc{}
	err := c.runCommandAndParse("show chassis routing-engine", &x)
	if err != nil {
		return nil, err
	}

	r := &routing_engine.RouteEngineStats{
		Temperature:        float64(x.Information.RouteEngine.Temperature.Value),
		MemoryUtilization:  float64(x.Information.RouteEngine.MemoryUtilization),
		CPUTemperature:     float64(x.Information.RouteEngine.CPUTemperature.Value),
		CPUUser:            float64(x.Information.RouteEngine.CPUUser),
		CPUBackground:      float64(x.Information.RouteEngine.CPUBackground),
		CPUSystem:          float64(x.Information.RouteEngine.CPUSystem),
		CPUInterrupt:       float64(x.Information.RouteEngine.CPUInterrupt),
		CPUIdle:            float64(x.Information.RouteEngine.CPUIdle),
		LoadAverageOne:     float64(x.Information.RouteEngine.LoadAverageOne),
		LoadAverageFive:    float64(x.Information.RouteEngine.LoadAAverageFive),
		LoadAverageFifteen: float64(x.Information.RouteEngine.LoadAverageFifteen),
	}

	return r, nil
}

func (c *RpcClient) EnvironmentItems() ([]*environment.EnvironmentItem, error) {
	var x = EnvironmentRpc{}
	err := c.runCommandAndParse("show chassis environment", &x)
	if err != nil {
		return nil, err
	}

	// remove duplicates
	list := make(map[string]float64)
	for _, item := range x.Information.Items {
		if item.Temperature != nil {
			list[item.Name] = float64(item.Temperature.Value)
		}
	}

	items := make([]*environment.EnvironmentItem, 0)
	for name, value := range list {
		i := &environment.EnvironmentItem{
			Name:        name,
			Temperature: value,
		}
		items = append(items, i)
	}

	return items, nil
}

func (c *RpcClient) InterfaceDiagnostics() ([]*interface_diagnostics.InterfaceDiagnostics, error) {
	var x = InterfaceDiagnosticsRpc{}
	err := c.runCommandAndParse("show interfaces diagnostics optics", &x)
	if err != nil {
		return nil, err
	}

	diagnostics := make([]*interface_diagnostics.InterfaceDiagnostics, 0)
	for _, diag := range x.Information.Diagnostics {
		if diag.Diagnostics.NA == "N/A" {
			continue
		}
		d := &interface_diagnostics.InterfaceDiagnostics{
			Name:              diag.Name,
			LaserBiasCurrent:  float64(diag.Diagnostics.LaserBiasCurrent),
			LaserOutputPower:  float64(diag.Diagnostics.LaserOutputPower),
			ModuleTemperature: float64(diag.Diagnostics.ModuleTemperature.Value),
		}
		f, err := strconv.ParseFloat(diag.Diagnostics.LaserOutputPowerDbm, 64)
		if err == nil {
			d.LaserOutputPowerDbm = f
		}

		if diag.Diagnostics.ModuleVoltage > 0 {
			d.ModuleVoltage = float64(diag.Diagnostics.ModuleVoltage)
			d.RxSignalAvgOpticalPower = float64(diag.Diagnostics.RxSignalAvgOpticalPower)
			f, err = strconv.ParseFloat(diag.Diagnostics.RxSignalAvgOpticalPowerDbm, 64)
			if err == nil {
				d.RxSignalAvgOpticalPowerDbm = f
			}
		} else {
			d.LaserRxOpticalPower = float64(diag.Diagnostics.LaserRxOpticalPower)
			f, err = strconv.ParseFloat(diag.Diagnostics.LaserRxOpticalPowerDbm, 64)
			if err == nil {
				d.LaserRxOpticalPowerDbm = f
			}
		}

		diagnostics = append(diagnostics, d)
	}

	return diagnostics, nil
}

func (c *RpcClient) runCommandAndParse(cmd string, obj interface{}) error {
	if c.debug {
		log.Printf("Running command on %s: %s\n", c.conn.Host, cmd)
	}

	b, err := c.conn.RunCommand(fmt.Sprintf("%s | display xml", cmd))
	if err != nil {
		return err
	}

	if c.debug {
		log.Printf("Output for %s: %s\n", c.conn.Host, string(b))
	}

	err = xml.Unmarshal(b, obj)
	return err
}
