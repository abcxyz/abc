**This is not an official Google product.**

# Template: NextJS React Template

This template was bootstrapped with [NextJS](https://nextjs.org/).

## Set up instruction
prerequisites: `node.js` and `npm`. If not, follow the download steps [here](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm).

1. cd into an empty directory

    ```shell
    $ mkdir ~/nextjs_react_template
    $ cd ~/nextjs_react_template
    ```
1. Install the `abc` [instructions](https://github.com/abcxyz/abc#user-guide)

1. Create a Regular Web Application in Auth0. These will generate values that are required to render this template.
1. In the Auth0 application settings, add the following callback URL, `<your_app_base_url>/api/auth/callback`
1. In the Auth0 application settings, add logout URL, `<your_app_base_url>`
1. Generate a random secret `openssl rand -hex 32`.
1. Execute the template defined in the `t` directory.
This will output a file named `main.go` in your working directory containing
the transformed program.

    ```shell
    $ abc templates render \
        -input=session_secret='RANDOM_SECRET' \
        -input=base_url='APP_URL' \
        -input=issuer_base_url='AUTH0_DOMAIN_URL' \
        -input=client_id='AUTH0_CLIENT_ID' \
        -input=client_secret='AUTH0_CLIENT_SECRET' \
        github.com/abcxyz/abc/t/nextjs_react_template@latest
    ```

### Template Inputs
- `session_secret` - [Required] A long secret value used to encrypt the session cookie.
- `base_url` - [Default: `http://localhost:3000` ] The base URL of the application.
- `issuer_base_url` - [Required] The base URL of the Auth0 tenant domain. Must include scheme (e.g. `http://`).
- `client_id` - [Required] The application Auth0 Client ID.
- `client_secret` - [Required] The application Auth0 Client Secret

## Available Scripts
In the project directory, you can run:

### `npm run dev`

Runs the app in the development mode. Open [http://localhost:3000](http://localhost:3000) to view it in your browser.

The page will reload when you make changes. You may also see any lint errors in the console.

### `npm run lint`

Apply static analysis and code format checks.

### `npm run fix`

Automatically fix the code problem with Eslint, Prettier and GTS.

### `npm run build` and `npm run start`

`npm run build` creates an optimized production build of your application. The output displays information about each route.

`npm run start` starts the application in production mode. The application should be compiled with next build first.
