
As described in [Design](design.md) document Backstage has 3 layers of configuration.
First layer is related to default configuration for all the instances inside Cluster and provided on Operator level. More details you can see in [Admin Guide](admin.md).
Other 2 are per instance (Custom Resource) configuration and described in this document.

** Raw Configuration



#### Custom Backstage Image

You can use the Backstage Operator to deploy a backstage application with your custom backstage image by setting the field `spec.application.image` in your Backstage CR. This is at your own risk and it is your responsibility to ensure that the image is from trusted sources, and has been tested and validated for security compliance.