# NextJS React Template

Template for a NextJS app integrated with Auth0.

### Installation
1. Create a Regular Web Application in Auth0. These will generate values that are required to render this template.
1. Generate a random secret `openssl rand -hex 32`.
1. Run the following command:

    ```shell
    $ abc templates render \
        -input=session_secret='RANDOM_SECRET' \
        -input=base_url='APP_URL' \
        -input=issuer_base_url='AUTH0_DOMAIN_URL' \
        -input=client_id='AUTH0_CLIENT_ID' \
        -input=client_secret='AUTH0_CLIENT_SECRET' \
        github.com/abcxyz/abc.git//t/nextjs_react_template
    ```

### Start development server
Follow the [installation](#installation) steps to render the template.

1. To download dependencies, run `npm install`
1. To start the server, run `npm run dev`

### Inputs
- `session_secret` - [Required] A long secret value used to encrypt the session cookie.
- `base_url` - [Default: `http://localhost:3000` ] The base URL of the application.
- `issuer_base_url` - [Required] The base URL of the Auth0 tenant domain. Must include scheme (e.g. `http://`).
- `client_id` - [Required] The application Auth0 Client ID.
- `client_secret` - [Required] The application Auth0 Client Secret

