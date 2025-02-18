package postgres

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/postgresql/mgmt/2020-01-01/postgresql"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/locks"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/postgres/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/postgres/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourcePostgreSQLConfiguration() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourcePostgreSQLConfigurationCreateUpdate,
		Read:   resourcePostgreSQLConfigurationRead,
		Delete: resourcePostgreSQLConfigurationDelete,
		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.ConfigurationID(id)
			return err
		}),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
			},

			"resource_group_name": azure.SchemaResourceGroupName(),

			"server_name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.ServerName,
			},

			"value": {
				Type:     pluginsdk.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourcePostgreSQLConfigurationCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Postgres.ConfigurationsClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	log.Printf("[INFO] preparing arguments for AzureRM PostgreSQL Configuration creation.")

	name := d.Get("name").(string)
	resGroup := d.Get("resource_group_name").(string)
	serverName := d.Get("server_name").(string)
	value := d.Get("value").(string)

	locks.ByName(serverName, postgreSQLServerResourceName)
	defer locks.UnlockByName(serverName, postgreSQLServerResourceName)

	properties := postgresql.Configuration{
		ConfigurationProperties: &postgresql.ConfigurationProperties{
			Value: utils.String(value),
		},
	}

	// NOTE: this resource intentionally doesn't support Requires Import
	//       since a fallback route is created by default

	future, err := client.CreateOrUpdate(ctx, resGroup, serverName, name, properties)
	if err != nil {
		return err
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return err
	}

	read, err := client.Get(ctx, resGroup, serverName, name)
	if err != nil {
		return err
	}
	if read.ID == nil {
		return fmt.Errorf("Cannot read PostgreSQL Configuration %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*read.ID)

	return resourcePostgreSQLConfigurationRead(d, meta)
}

func resourcePostgreSQLConfigurationRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Postgres.ConfigurationsClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.ConfigurationID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.ResourceGroup, id.ServerName, id.Name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[WARN] PostgreSQL Configuration '%s' was not found (resource group '%s')", id.Name, id.ResourceGroup)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("making Read request on Azure PostgreSQL Configuration %s: %+v", id.Name, err)
	}

	d.Set("name", id.Name)
	d.Set("server_name", id.ServerName)
	d.Set("resource_group_name", id.ResourceGroup)

	if props := resp.ConfigurationProperties; props != nil {
		d.Set("value", props.Value)
	}

	return nil
}

func resourcePostgreSQLConfigurationDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).Postgres.ConfigurationsClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.ConfigurationID(d.Id())
	if err != nil {
		return err
	}

	locks.ByName(id.ServerName, postgreSQLServerResourceName)
	defer locks.UnlockByName(id.ServerName, postgreSQLServerResourceName)

	// "delete" = resetting this to the default value
	resp, err := client.Get(ctx, id.ResourceGroup, id.ServerName, id.Name)
	if err != nil {
		return fmt.Errorf("retrieving Postgresql Configuration '%s': %+v", id.Name, err)
	}

	properties := postgresql.Configuration{
		ConfigurationProperties: &postgresql.ConfigurationProperties{
			// we can alternatively set `source: "system-default"`
			Value: resp.DefaultValue,
		},
	}

	future, err := client.CreateOrUpdate(ctx, id.ResourceGroup, id.ServerName, id.Name, properties)
	if err != nil {
		return err
	}

	if err = future.WaitForCompletionRef(ctx, client.Client); err != nil {
		return err
	}

	return nil
}
