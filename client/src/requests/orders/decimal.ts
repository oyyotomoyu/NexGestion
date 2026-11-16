export function scaledDecimal(value: string): bigint|null {
 if(!/^\d+(\.\d{1,6})?$/.test(value))return null;
 const [whole,fraction=""]=value.split(".");return BigInt(whole)*1000000n+BigInt(fraction.padEnd(6,"0"));
}
function formatScaled(value: bigint) { const whole=value/1000000n;const fraction=(value%1000000n).toString().padStart(6,"0").replace(/0+$/,"");return `${whole}${fraction?`.${fraction}`:""}`; }
export function draftTotal(lines: {quantity:string;unit_price:string}[]) {
 let sum=0n;for(const line of lines){const q=scaledDecimal(line.quantity),p=scaledDecimal(line.unit_price);if(q===null||p===null)return "—";sum+=q*p}return formatScaled((sum+500000n)/1000000n);
}
