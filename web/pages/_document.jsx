import { Html, Head, Main, NextScript } from 'next/document'

export default function Document() {
  return (
    <Html lang="en" className="no-js">
      <Head>
        <script
          dangerouslySetInnerHTML={{
            __html: "document.documentElement.className=document.documentElement.className.replace(/\\bno-js\\b/,'js');",
          }}
        />
      </Head>
      <body>
        <Main />
        <NextScript />
      </body>
    </Html>
  );
}
