package app

import (
	"context"

	"github.com/cyverse-de/go-mod/pbinit"
	"github.com/cyverse-de/p/go/qms"
	"github.com/cyverse-de/subscriptions/db"
	"github.com/cyverse-de/subscriptions/errors"
)

func (a *App) AddQuotaHandler(subject, reply string, request *qms.AddQuotaRequest) {
	var err error

	log := log.WithField("context", "add quota")

	sendError := func(ctx context.Context, response *qms.QuotaResponse, err error) {
		log.Error(err)
		response.Error = errors.NatsError(ctx, err)
		if err = a.client.Respond(ctx, reply, response); err != nil {
			log.Error(err)
		}
	}

	response := pbinit.NewQuotaResponse()

	ctx, span := pbinit.InitQMSAddQuotaRequest(request, subject)
	defer span.End()

	subscriptionID := request.Quota.SubscriptionId

	d := db.New(a.db)

	_, update, err := d.GetCurrentQuota(ctx, request.Quota.ResourceType.Uuid, subscriptionID)
	if err != nil {
		sendError(ctx, response, err)
		return
	}

	if err = d.UpsertQuota(ctx, update, float64(request.Quota.Quota), request.Quota.ResourceType.Uuid, subscriptionID); err != nil {
		sendError(ctx, response, err)
		return
	}

	value, _, err := d.GetCurrentQuota(ctx, request.Quota.ResourceType.Uuid, subscriptionID)
	if err != nil {
		sendError(ctx, response, err)
		return
	}

	response.Quota = &qms.Quota{
		Quota:          value,
		ResourceType:   request.Quota.ResourceType,
		SubscriptionId: subscriptionID,
	}

	if err = a.client.Respond(ctx, reply, response); err != nil {
		log.Error(err)
	}
}
