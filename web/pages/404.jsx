import Image from "next/image";

import cx from 'classnames';

import { Literata } from 'next/font/google';
import { useRouter } from "next/router";

const literata = Literata({
  subsets: ['latin'],
  weight: '500',
});

export default function NotFoundPage() {
  const router = useRouter();

  return (
    <>
      <style jsx>{`
        .fade-in-1sdelay {
          opacity: 0;
          animation: fadeInAnimation 700ms ease-in 500ms forwards;
        }

        .fade-in-3sdelay {
          opacity: 0;
          animation: fadeInAnimation 700ms ease-in 1200ms forwards;
        }

        .fade-in-5sdelay {
          opacity: 0;
          animation: fadeInAnimation 700ms ease-in 1900ms forwards;
        }

        .spinChipImg {
          animation: spin 40s linear infinite;
          transform: translateZ(0);
        }

        @keyframes fadeInAnimation {
          0% {
            opacity: 0;
          }
          100% {
            opacity: 1;
          }
        }

        @keyframes spin {
          from {
            transform: rotate(360deg);
          }
          to {
            transform: rotate(0deg);
          }
        }
      `}</style>
      <div
        className={literata.className}
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '2.5rem',
        }}
      >
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            placeItems: 'center',
            justifyContent: 'space-between',
            gap: '15px',
            border: '7px double',
            borderRadius: '5px',
            width: '250px',
            height: '350px',
            backgroundColor: '#fbfbfb',
            fontSize: '2rem',
          }}
        >
          <h1
            className="fade-in-1sdelay"
            style={{
              alignSelf: 'flex-start',
              paddingLeft: '15px',
            }}
          >
            4
          </h1>
          <div className="fade-in-3sdelay">
            <div className="spinChipImg">
              <Image
                src='/pokerchip.png'
                width={100}
                height={100}
                alt='0'
              />
            </div>
          </div>
          <h1
            className="fade-in-5sdelay"
            style={{
              alignSelf: 'flex-end',
              paddingRight: '15px',
            }}
          >
            4
          </h1>
        </div>
        <p style={{ padding: '10px' }}>page not found</p>
        <button
          style={{
            padding: '7px',
            marginTop: '10px',
            fontSize: '1.1rem',
          }}
          onClick={() => {
            router.push('/');
          }}
        >
          go home
        </button>
      </div>
    </>
  );
}
