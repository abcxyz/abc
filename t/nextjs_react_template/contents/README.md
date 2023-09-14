**This is not an official Google product.**

# Template: NextJS React Template

This template was bootstrapped with [NextJS](https://nextjs.org/). 

## Set up instruction

1. cd into an empty directory

    ```shell
    $ mkdir ~/nextjs_react_template
    $ cd ~/nextjs_react_template
    ```
1. Install the `abc` [instructions](https://github.com/abcxyz/abc#user-guide)

1. Execute the template defined in the `t` directory.
This will output a file named `main.go` in your working directory containing
the transformed program.

    ```shell
    $ abc templates render github.com/abcxyz/abc.git//t/nextjs_react_template
    ```

## Available Scripts
prerequisites: `node.js` and `npm`. If not, follow the download steps [here](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm).

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
