package supervisor

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/jakekeeys/givforecast/internal/assist"
)

const (
	HA_SOC_ENTITY_ID                 = "sensor.battery_soc"
	HA_SOC_TARGET_ENTITY_ID          = "number.givtcp_ems2503039_ems_charge_target_soc_1"
	HA_CHARGE_PERIOD_START_ENTITY_ID = "select.givtcp_ems2503039_ems_charge_start_time_slot_1"
	HA_CHARGE_PERIOD_END_ENTITY_ID   = "select.givtcp_ems2503039_ems_charge_end_time_slot_1"

	HA_CHARGE_PERIOD_START_OPTION_VALUE = "01:30:00"
	HA_CHARGE_PERIOD_END_OPTION_VALUE   = "08:30:00"
)

type Supervisor struct {
	cfg Config
	ctx context.Context
	hac *assist.Client
}

type Config struct {
	PollInterval time.Duration
}

func New(cfg Config, ctx context.Context, hac *assist.Client) (*Supervisor, error) {
	return &Supervisor{
		cfg: cfg,
		ctx: ctx,
		hac: hac,
	}, nil
}

func (s *Supervisor) Start() {
	go func() {
		slog.Info("starting supervisor")
		err := s.poll()
		if err != nil {
			slog.Error("failed to poll", err)
		}
		for {
			select {
			case <-time.After(s.cfg.PollInterval):
				err := s.poll()
				if err != nil {
					slog.Error("failed to poll", err)
				}
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *Supervisor) poll() error {
	now := time.Now().UTC()
	chargingPeriodStart := time.Date(now.Year(), now.Month(), now.Day(), 1, 30, 0, 0, time.UTC)
	chargingPeriodEnd := time.Date(now.Year(), now.Month(), now.Day(), 8, 30, 0, 0, time.UTC)

	if now.Before(chargingPeriodStart) || now.After(chargingPeriodEnd) {
		slog.Debug("not in charging period")
		return nil
	}

	state, err := s.getState()
	if err != nil {
		return err
	}

	switch {
	case state.SOC <= state.Target && !s.chargingEnabled(state):
		err := s.enableCharging()
		if err != nil {
			return err
		}
		slog.Info("enabled charging")
	case state.SOC > state.Target && s.chargingEnabled(state):
		err := s.disableCharging()
		if err != nil {
			return err
		}
		slog.Info("disabled charging")
	default:
		slog.Debug("no action required")
	}

	return nil
}

type State struct {
	Target int
	SOC    int
	Start  string
	End    string
}

func (s *Supervisor) chargingEnabled(state *State) bool {
	return !(state.Start == state.End)
}

func (s *Supervisor) enableCharging() error {
	return s.hac.SetSelectOption(HA_CHARGE_PERIOD_END_ENTITY_ID, HA_CHARGE_PERIOD_END_OPTION_VALUE)
}

func (s *Supervisor) disableCharging() error {
	return s.hac.SetSelectOption(HA_CHARGE_PERIOD_END_ENTITY_ID, HA_CHARGE_PERIOD_START_OPTION_VALUE)
}

func (s *Supervisor) getState() (*State, error) {
	target, err := s.hac.GetState(HA_SOC_TARGET_ENTITY_ID)
	if err != nil {
		return nil, err
	}
	targetFloat, err := strconv.ParseFloat(target.State, 64)
	if err != nil {
		return nil, err
	}

	soc, err := s.hac.GetState(HA_SOC_ENTITY_ID)
	if err != nil {
		return nil, err
	}
	socFloat, err := strconv.ParseFloat(soc.State, 64)
	if err != nil {
		return nil, err
	}

	start, err := s.hac.GetState(HA_CHARGE_PERIOD_START_ENTITY_ID)
	if err != nil {
		return nil, err
	}

	end, err := s.hac.GetState(HA_CHARGE_PERIOD_END_ENTITY_ID)
	if err != nil {
		return nil, err
	}

	return &State{
		Target: int(targetFloat),
		SOC:    int(socFloat),
		Start:  start.State,
		End:    end.State,
	}, nil
}
