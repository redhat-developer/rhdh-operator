# External DB integration

Backstage hosts the data in a [PostgreSQL database](https://backstage.io/docs/getting-started/config/database/).
By default, the Operator creates and manages a local instance of PostgreSQL in the same namespace as the Backstage deployment but it also allows to switch this off and configure an external database server instead.
Usually, external connection requires more security, so, this instruction includes steps to configure SSL/TLS.

### Configure your external PostgreSQL instance
As a prerequisite, you have to know:
- **db-host** - your PostgreSQL instance DNS or IP address 
- **db-port** - your PostgreSQL instance port number (usually 5432)
- **username** - to connect to your PostgreSQL instance
- **password** - to connect to your PostgreSQL instance

**NOTE:** By default, Backstage uses databases for each plugin and automatically creates them if none are found, so in addition to PSQL Database level privileges, the user may need Create Database privilege.  

In addition, to get your database connection secured with SSL/TLS, you also need certificates in the form of PEM file. 

You can find configuration guidelines for:
- [AWS RDS PostgreSQL](#aws-rds-postgresql)
- [Azure Database PostgreSQL](#azure-db-postgresql)

### Create secret with PostgreSQL connection properties:
````yaml
cat <<EOF | kubectl -n <your-namespace> create -f -
apiVersion: v1
kind: Secret
metadata:
 name: <cred-secret-name>
type: Opaque
stringData:
 POSTGRES_PASSWORD: <password>
 POSTGRES_PORT: <db-port>
 POSTGRES_USER: <username>
 POSTGRES_HOST: <db-host>
 PGSSLMODE: require #  for TLS connection
 NODE_EXTRA_CA_CERTS: <abs-path-to-pem-file> # for TLS connection, e.g. /opt/app-root/src/postgres-crt.pem
EOF
````

### Create secret with certificate(s):
(omit this step if you do not need TLS connection, maybe for testing purpose)

````yaml
cat <<EOF | kubectl -n <your-namespace> create -f -
apiVersion: v1
kind: Secret
metadata:
 name: <crt-secret>
type: Opaque
stringData:
 postgres-crt.pem: |-
   -----BEGIN CERTIFICATE-----
   MIIFqDCCA5CgAwIBAgIQHtOXCV/YtLNHcB6qvn9FszANBgkqhkiG9w0BAQwFADBl
   ... 
````

### Create Backstage Custom Resource:

- disable creating local PostgreSQL instance with **spec.database.enableLocalDb: false**
- add **<crt-secret>** to **spec.application.extraFiles.secrets**, so, as for example below **postgres-crt.pem** file will be mounted to Backstage container at **spec.application.extraFiles.mountPath** directory:   
- add **<cred-secret>** to **spec.application.extraEnvs.secrets**, so all the data's entries will be injected to Backstage container as environment variables.

**NOTE:** environment variables listed in **<cred-secret>** file work with default Operator configuration. If it is changed on default or raw configuration, you have to re-configure it accordingly.

````yaml
cat <<EOF | kubectl -n <your-namespace> create -f -
apiVersion: rhdh.redhat.com/v1alpha1
kind: Backstage
metadata:
 name: <backstage-instance-name>
spec:
 database:
   enableLocalDb: false 
 application: 
    extraFiles:
     mountPath: <path> # e g /opt/app-root/src
     secrets:
       - name: <crt-secret> 
         key: postgres-crt.pem # key name as in <crt-secret> Secret
    extraEnvs:
      secrets:
        - name: <cred-secret>  
````

## External PostgreSQL types

### AWS RDS PostgreSQL
(Tested on PGSQL 15)

#### Prerequisites
- An AWS account with an active subscription and a PostgreSQL instance on [Amazon RDS for PostgreSQL](https://aws.amazon.com/rds/postgresql/) 
- (Optionally) Pgsql client installed to check your database connections 

#### Preparation
- (Optionally) Check your Database connection:

````
psql -h <db-host> -p <db-port> -U <username>
````

- Enter the <password> and output should be something like:

````
SSL connection (protocol: TLSv1.3, cipher: TLS_AES_256_GCM_SHA384, compression: off)
Type "help" for help.
postgres=>
````

(type ‘\q’ to quit from psql CLI)

**TIP:** The most probable reason for an unsuccessful connection is not properly configured Security Group inbound rule. Make sure you have one enabled for external connection.

- Download a certificate bundle [Certificate bundles for all AWS Regions](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.SSL.html#UsingWithRDS.SSL.CertificatesAllRegions) or [Certificate bundles for specific AWS Region](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.SSL.html#UsingWithRDS.SSL.RegionCertificates).

**NOTE:**  AWS RDS **enforces** connecting your client applications using Transport Layer Security (TLS) starting from PGSQL v 15 . You can disable it adding Parameter Group and setting **rds.force-ssl=0**

- Use this PEM file (**postgres-crt.pem**) as a data for the  **<crt-secret>** Secret above.

### Azure DB PostgreSQL
(Tested on PGSQL 15)

#### Prerequisites
- An [Azure](https://azure.microsoft.com/) account with an active subscription and [Azure Database for PostgreSQL - Flexible Server instance](https://learn.microsoft.com/en-gb/azure/postgresql/flexible-server/overview).
- (Optionally) Pgsql client installed to check your database connections 

#### Preparation

- (Optionally) Check your Database connection:

````
psql -h <db-host> -p <db-port> -U <username>
````

Enter the <password> and output should be something like:
````
SSL connection (protocol: TLSv1.3, cipher: TLS_AES_256_GCM_SHA384, compression: off)
Type "help" for help.
postgres=>
````
(type ‘\q’ to quit from psql CLI)

**TIP**: The most probable reason for an unsuccessful connection is not appropriate for public access Firewall rules.


- Download Microsoft RSA Root Certificate Authority 2017 and DigiCert Global Root CA certificates from the URIs provided [here](https://learn.microsoft.com/en-gb/azure/postgresql/flexible-server/concepts-networking-ssl-tls#downloading-root-ca-certificates-and-updating-application-clients-in-certificate-pinning-scenarios)

**NOTE:**  Azure Database for PostgreSQL flexible server **enforces** connecting your client applications using Transport Layer Security (TLS)

- Convert .crt files you downloaded to .pem format as [suggested](https://learn.microsoft.com/en-gb/azure/postgresql/flexible-server/concepts-networking-ssl-tls#downloading-root-ca-certificates-and-updating-application-clients-in-certificate-pinning-scenarios) using

````
openssl x509 -in DigiCertGlobalRootCA.crt -out DigiCertGlobalRootCA.crt.pem -outform PEM

openssl x509 -in "Microsoft ECC Root Certificate Authority 2017.crt" -out "Microsoft ECC Root Certificate Authority 2017.crt.pem" -outform PEM
````

- Combine them according to this [suggestion](https://learn.microsoft.com/en-gb/azure/postgresql/flexible-server/how-to-update-client-certificates-java#updating-root-ca-certificates-for-other-clients-for-certificate-pinning-scenarios), like:

````
cat DigiCertGlobalRootCA.crt.pem "Microsoft ECC Root Certificate Authority 2017.crt.pem" > postgres-crt.pem
````