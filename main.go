package main

import (
	"log"
	"strconv"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hashicorp/go-plugin"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"
	"github.com/victorcoder/dkron/dkron"
	dkplugin "github.com/victorcoder/dkron/plugin"
)

// maxBackoffTime waits up to x before giving up connecting to the RabbitMQ service
// TODO currently, dkron waits for a plugin to connect for 1 minutes before giving
// up. Waiting for 2 minutes will prevent dkron from starting up and if run in
// kubernetes it should reboot the pod and try again.
var maxBackoffTime = time.Minute * 2

type rabbitMQExecutor struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func createRabbitMQExecutor() (*rabbitMQExecutor, error) {

	executor := &rabbitMQExecutor{}
	if err := executor.connect(); err != nil {
		return nil, err
	}

	return executor, nil
}

func (s *rabbitMQExecutor) connect() error {
	connectionString := viper.GetString("rabbit_host")
	log.Printf("[rabbitMQExecutor] connecting to rabbit [%s]", connectionString)

	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = maxBackoffTime
	return backoff.RetryNotify(func() error {
		conn, err := amqp.Dial(connectionString)
		if err != nil {
			log.Printf("[rabbitMQExecutor] failed connecting to rabbitmq [%s]", connectionString)
			return err
		}

		ch, err := conn.Channel()
		if err != nil {
			log.Printf("[rabbitMQExecutor] failed to open channel")
			return err
		}

		s.conn = conn
		s.ch = ch
		return nil
	}, b, func(err error, t time.Duration) {
		log.Printf("[rabbitMQExecutor] failed setting up RabbitMQ executor - trying again [%s]", connectionString)
	})

}

func fetchConfig(config map[string]string, key string, defaultValue string) string {

	if value, ok := config[key]; ok {
		return value
	}

	return defaultValue
}

func (s *rabbitMQExecutor) publish(args *dkron.ExecuteRequest) error {
	mandatory, err := strconv.ParseBool(fetchConfig(args.Config, "mandatory", "false"))
	if err != nil {
		log.Printf("[rabbitMQExecutor] [%s] 'mandatory' have invalid value - %v", args.JobName, err)
	}

	immediate, err := strconv.ParseBool(fetchConfig(args.Config, "immediate", "false"))
	if err != nil {
		log.Printf("[rabbitMQExecutor] [%s] 'immediate' have invalid value - %v", args.JobName, err)
	}

	return s.ch.Publish(
		fetchConfig(args.Config, "exchange", ""),
		args.Config["queue_name"],
		mandatory,
		immediate,
		amqp.Publishing{
			ContentType: fetchConfig(args.Config, "header", "application/json"),
			Body:        []byte(args.Config["payload"]),
		})

}
func (s *rabbitMQExecutor) Execute(args *dkron.ExecuteRequest) ([]byte, error) {
	queueName := args.Config["queue_name"]
	log.Printf("[rabbitMQExecutor] [%s] will publish to queue [%s]\n", args.JobName, queueName)

	err := s.publish(args)
	if err == amqp.ErrClosed {
		log.Printf("[rabbitMQExecutor] [%s] got closed error while trying to publish, trying to reconnect\n", args.JobName)
		if err = s.connect(); err != nil {
			log.Printf("[rabbitMQExecutor] [%s] failed to reconnect [%s]\n", args.JobName, err)
			return []byte(err.Error()), err
		}

		err = s.publish(args)
	}
	if err != nil {
		return []byte(err.Error()), err
	}

	return []byte("OK"), nil
}

func main() {

	viper.SetDefault("rabbit_host", "amqp://guest:guest@localhost:5672/")

	viper.SetConfigName("dkron-executor-rabbitmq") // name of config file (without extension)
	viper.AddConfigPath("/etc/dkron")              // call multiple times to add many search paths
	viper.AddConfigPath("$HOME/.dkron")            // call multiple times to add many search paths
	viper.AddConfigPath("./config")                // call multiple times to add many search paths
	viper.SetEnvPrefix("dkron_executor_rabbitmq")  // will be uppercased automatically
	viper.AutomaticEnv()

	viper.ReadInConfig()

	executor, err := createRabbitMQExecutor()
	if err != nil {
		log.Printf("Failed to create rabbit mq executor, error: %v", err)
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: dkplugin.Handshake,
		Plugins: map[string]plugin.Plugin{
			"executor": &dkplugin.ExecutorPlugin{Executor: executor},
		},

		// A non-nil value here enables gRPC serving for this plugin...
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
