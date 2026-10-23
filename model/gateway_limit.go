package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/gatewaylimit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var gatewayLimitMutex sync.Mutex
var ErrGatewayLimitConflict = errors.New("rate limit rules were modified; refresh before saving")
var ErrGatewayLimitInvalid = errors.New("invalid rate limit policy")

func SaveGatewayLimits(config gatewaylimit.Config) error {
	compiled, err := gatewaylimit.Compile(config)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGatewayLimitInvalid, err)
	}
	gatewayLimitMutex.Lock()
	defer gatewayLimitMutex.Unlock()
	err = DB.Transaction(func(tx *gorm.DB) error {
		initial := Option{Key: gatewaylimit.OptionKey, Value: `{"revision":0,"rules":[]}`}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		var row Option
		if err := lockForUpdate(tx).Where(&Option{Key: gatewaylimit.OptionKey}).First(&row).Error; err != nil {
			return err
		}
		var previous gatewaylimit.Config
		if err := common.UnmarshalJsonStr(row.Value, &previous); err != nil {
			return err
		}
		if previous.Revision != config.Revision {
			return ErrGatewayLimitConflict
		}
		old := map[string]gatewaylimit.Rule{}
		for _, r := range previous.Rules {
			old[r.ID] = r
		}
		users := map[int]bool{}
		tokens := map[int]bool{}
		channels := map[int]bool{}
		for _, r := range compiled.Config.Rules {
			if before, found := old[r.ID]; found && (r.Scope != before.Scope || r.SubjectID != before.SubjectID || r.Model != before.Model || !slices.Equal(r.ChannelIDs, before.ChannelIDs)) {
				return fmt.Errorf("%w: a rule's target cannot be changed; create a new rule", ErrGatewayLimitInvalid)
			}
			if before, found := old[r.ID]; found && (before.Enabled || !r.Enabled) {
				continue
			}
			switch r.Scope {
			case "user", "user_model":
				users[r.SubjectID] = true
			case "api_key":
				tokens[r.SubjectID] = true
			case "provider":
				for _, id := range r.ChannelIDs {
					channels[id] = true
				}
			}
		}
		for _, target := range []struct {
			model any
			ids   map[int]bool
		}{{&User{}, users}, {&Token{}, tokens}, {&Channel{}, channels}} {
			if len(target.ids) == 0 {
				continue
			}
			ids := make([]int, 0, len(target.ids))
			for id := range target.ids {
				ids = append(ids, id)
			}
			var count int64
			if err := tx.Model(target.model).Where("id IN ?", ids).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return fmt.Errorf("%w: user, API key or channel does not exist", ErrGatewayLimitInvalid)
			}
		}
		compiled.Config.Revision = previous.Revision + 1
		raw, err := common.Marshal(compiled.Config)
		if err != nil {
			return err
		}
		return tx.Model(&Option{}).Where(&Option{Key: gatewaylimit.OptionKey}).Update("value", string(raw)).Error
	})
	if err != nil {
		return err
	}
	raw, err := common.Marshal(compiled.Config)
	if err != nil {
		return err
	}
	if err := updateOptionMap(gatewaylimit.OptionKey, string(raw)); err != nil {
		return err
	}
	// Pub/sub accelerates propagation; existing database option refresh is the recovery path.
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := common.RDB.Publish(ctx, gatewaylimit.PolicyTopic, string(raw)).Err(); err != nil {
			common.SysError("gateway limit policy notification failed: " + err.Error())
		}
	}
	return nil
}

func applyGatewayLimits(raw string) error {
	var config gatewaylimit.Config
	if strings.TrimSpace(raw) != "" {
		if err := common.UnmarshalJsonStr(raw, &config); err != nil {
			return err
		}
	}
	policy, err := gatewaylimit.Compile(config)
	if err == nil {
		gatewaylimit.Publish(policy)
	}
	return err
}

func ListenGatewayLimitPolicies(ctx context.Context) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	subscription := common.RDB.Subscribe(ctx, gatewaylimit.PolicyTopic)
	defer subscription.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-subscription.Channel():
			if !ok {
				return
			}
			gatewayLimitMutex.Lock()
			if err := applyGatewayLimits(message.Payload); err != nil {
				common.SysError("invalid gateway limit policy notification: " + err.Error())
			}
			gatewayLimitMutex.Unlock()
		}
	}
}
