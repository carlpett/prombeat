package beater

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/carlpett/prombeat/config"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/publisher"
	"github.com/prometheus/client_golang/api/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

type Prombeat struct {
	done            chan struct{}
	config          config.Config
	client          publisher.Client
	promQueryClient prometheus.QueryAPI
	name            string
}

func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	config := config.DefaultConfig
	if err := cfg.Unpack(&config); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	promcfg := prometheus.Config{
		Address: config.Address,
	}
	prometheusClient, err := prometheus.New(promcfg)
	if err != nil {
		return nil, err
	}
	queryClient := prometheus.NewQueryAPI(prometheusClient)

	bt := &Prombeat{
		done:            make(chan struct{}),
		config:          config,
		promQueryClient: queryClient,
		name:            b.Name,
	}
	return bt, nil
}

func (bt *Prombeat) Run(b *beat.Beat) error {
	logp.Info("prombeat is running! Hit CTRL-C to stop it.")

	bt.client = b.Publisher.Connect()
	ticker := time.NewTicker(bt.config.Period)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		select {
		case <-bt.done:
			// Deferred cancel will be called here
			return nil
		case <-ticker.C:
		}

		var wg sync.WaitGroup

		for _, query := range bt.config.Queries {
			wg.Add(1)
			go func(query config.Query) {
				err := bt.executeQuery(query, ctx)
				if err != nil {
					logp.Warn("%v", err)
				}

				wg.Done()
			}(query)
		}

		samples, err := bt.executeFederation(bt.config.Matchers)
		if err != nil {
			logp.Warn("%v", err)
		}
		for _, sample := range samples {
			wg.Add(1)
			go func(sample model.Vector) {
				for _, elem := range sample {
					name := string(elem.Metric[model.MetricNameLabel])
					event := common.MapStr{
						"@timestamp": common.Time(elem.Timestamp.Time()),
						"type":       bt.name,
						"labels":     elem.Metric,
						name:         elem.Value,
					}
					bt.client.PublishEvent(event)
				}
			}(sample)
			wg.Done()
		}

		wg.Wait()
	}
}

func (bt *Prombeat) executeFederation(matchers []string) (samples []model.Vector, err error) {
	endpoint := fmt.Sprintf("%s/federate", bt.config.Address)
	values := url.Values{}
	for _, matcher := range matchers {
		values.Add("match[]", matcher)
	}
	url := fmt.Sprintf("%s?%s", endpoint, values.Encode())

	r, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	textDecoder := expfmt.NewDecoder(r.Body, expfmt.FmtText)
	dec := expfmt.SampleDecoder{
		Dec: textDecoder,
	}
	for {
		var sample model.Vector
		err = dec.Decode(&sample)

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		samples = append(samples, sample)
	}

	return samples, nil
}

func (bt *Prombeat) executeQuery(query config.Query, ctx context.Context) error {
	val, err := bt.promQueryClient.Query(ctx, query.Query, time.Now())
	if err != nil {
		return fmt.Errorf("Query %s returned an error from Prometheus: %v\n", query.Name, err)
	}

	sentEvents := 0
	switch {
	case val.Type() == model.ValScalar:
		scalarVal := val.(*model.Scalar)
		event := common.MapStr{
			"@timestamp": common.Time(scalarVal.Timestamp.Time()),
			"type":       bt.name,
			query.Name:   scalarVal.Value,
		}
		bt.client.PublishEvent(event)
		sentEvents++

	case val.Type() == model.ValVector:
		vectorVal := val.(model.Vector)
		for _, elem := range vectorVal {
			event := common.MapStr{
				"@timestamp": common.Time(elem.Timestamp.Time()),
				"type":       bt.name,
				"labels":     elem.Metric,
				query.Name:   elem.Value,
			}
			bt.client.PublishEvent(event)
			sentEvents++
		}

	case val.Type() == model.ValMatrix:
		matrixVal := val.(model.Matrix)
		for _, sampleStream := range matrixVal {
			for _, sample := range sampleStream.Values {
				event := common.MapStr{
					"@timestamp": common.Time(sample.Timestamp.Time()),
					"type":       bt.name,
					"labels":     sampleStream.Metric,
					query.Name:   sample.Value,
				}
				bt.client.PublishEvent(event)
				sentEvents++
			}
		}

	case val.Type() == model.ValString:
		stringVal := val.(*model.String)
		event := common.MapStr{
			"@timestamp": common.Time(stringVal.Timestamp.Time()),
			"type":       bt.name,
			query.Name:   stringVal.Value,
		}
		bt.client.PublishEvent(event)
		sentEvents++

	case val.Type() == model.ValNone:
		return fmt.Errorf("Query %q returned value of type None\n", query.Name)
	}

	logp.Info("Sent %d events for query %s", sentEvents, query.Name)
	return nil
}

func (bt *Prombeat) Stop() {
	bt.client.Close()
	close(bt.done)
}
