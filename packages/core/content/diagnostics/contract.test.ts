import {describe,it,expect} from "vitest";
import {pageSchema,parseDiagnostic,runSchema,mergeEvents} from "./contract";
describe("diagnostic API contracts",()=>{
 it("rejects malformed success and absent run evidence",()=>{expect(()=>parseDiagnostic({events:"ok",cursor:1},pageSchema)).toThrow("OUTPUT_SCHEMA");expect(()=>parseDiagnostic({regression:"passed"},runSchema)).toThrow()});
 it("accepts additive fields without inventing results",()=>{expect(parseDiagnostic({events:[],cursor:0,gap:false,has_more:false,future:true},pageSchema).events).toEqual([]);expect(mergeEvents([],[])).toEqual([])});
});
